package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestStartConversationEnforcesListingRules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		listing domain.Listing
		user    domain.User
		wantErr error
	}{
		{name: "active", listing: chatListing(domain.ListingStatusActive, domain.ModerationStatusVisible), user: domain.User{ID: 2, Status: domain.UserStatusActive}},
		{name: "reserved", listing: chatListing(domain.ListingStatusReserved, domain.ModerationStatusVisible), user: domain.User{ID: 2, Status: domain.UserStatusActive}},
		{name: "sold", listing: chatListing(domain.ListingStatusSold, domain.ModerationStatusVisible), user: domain.User{ID: 2, Status: domain.UserStatusActive}, wantErr: domain.ErrListingUnavailable},
		{name: "archived", listing: chatListing(domain.ListingStatusArchived, domain.ModerationStatusVisible), user: domain.User{ID: 2, Status: domain.UserStatusActive}, wantErr: domain.ErrListingUnavailable},
		{name: "removed", listing: chatListing(domain.ListingStatusActive, domain.ModerationStatusRemoved), user: domain.User{ID: 2, Status: domain.UserStatusActive}, wantErr: domain.ErrListingUnavailable},
		{name: "self", listing: chatListing(domain.ListingStatusActive, domain.ModerationStatusVisible), user: domain.User{ID: 1, Status: domain.UserStatusActive}, wantErr: domain.ErrSelfConversation},
		{name: "disabled", listing: chatListing(domain.ListingStatusActive, domain.ModerationStatusVisible), user: domain.User{ID: 2, Status: domain.UserStatusDisabled}, wantErr: domain.ErrUserDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repository := &chatRepository{conversation: domain.Conversation{ID: 9, ListingID: 100, SellerID: 1, BuyerID: 2}}
			service := app.NewChatService(chatListingReader{listing: tt.listing}, repository, nil, func() time.Time { return now })
			conversation, created, err := service.StartConversation(context.Background(), tt.user, 100)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StartConversation() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if repository.startCalled {
					t.Error("repository was called for rejected conversation")
				}
				return
			}
			if !repository.startCalled || !created || conversation.ID != 9 || !repository.start.CreatedAt.Equal(now) {
				t.Errorf("StartConversation() = %+v, %v; params = %+v", conversation, created, repository.start)
			}
		})
	}
}

func TestSendMessageCommitsBeforeBestEffortPublish(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repository := &chatRepository{
		message:      domain.Message{ID: 7, ConversationID: 3, SenderID: 2, Body: "hello", CreatedAt: now},
		conversation: domain.Conversation{ID: 3, SellerID: 1, BuyerID: 2},
	}
	publisher := &messagePublisher{}
	service := app.NewChatService(chatListingReader{}, repository, publisher, func() time.Time { return now })
	message, err := service.SendMessage(context.Background(), domain.User{ID: 2, Status: domain.UserStatusActive}, 3, "  hello  ")
	if err != nil || message.ID != 7 {
		t.Fatalf("SendMessage() = %+v, %v", message, err)
	}
	if repository.create.Body != "hello" || len(publisher.events) != 1 {
		t.Fatalf("stored/published = %+v/%+v", repository.create, publisher.events)
	}
	event := publisher.events[0]
	if event.ConversationID != 3 || event.MessageID != 7 || event.SellerID != 1 || event.BuyerID != 2 {
		t.Errorf("event = %+v", event)
	}

	repository.createErr = errors.New("database unavailable")
	publisher.events = nil
	_, err = service.SendMessage(context.Background(), domain.User{ID: 2, Status: domain.UserStatusActive}, 3, "still durable")
	if err == nil || len(publisher.events) != 0 {
		t.Errorf("persistence failure = %v, published = %+v", err, publisher.events)
	}
}

func TestListMessagesReordersLatestPageAndRejectsConflictingDirections(t *testing.T) {
	t.Parallel()

	repository := &chatRepository{
		conversation:   domain.Conversation{ID: 3, SellerID: 1, BuyerID: 2},
		beforeMessages: []domain.Message{{ID: 3}, {ID: 2}},
	}
	service := app.NewChatService(chatListingReader{}, repository, nil, time.Now)
	messages, err := service.ListMessages(context.Background(), 1, 3, app.MessageOptions{})
	if err != nil || len(messages) != 2 || messages[0].ID != 2 || messages[1].ID != 3 {
		t.Fatalf("ListMessages() = %+v, %v", messages, err)
	}
	before, after := int64(2), int64(3)
	_, err = service.ListMessages(context.Background(), 1, 3, app.MessageOptions{BeforeID: &before, AfterID: &after})
	var query *domain.QueryError
	if !errors.As(err, &query) {
		t.Errorf("combined directions error = %v", err)
	}
}

func TestMarkConversationReadUsesParticipantConversation(t *testing.T) {
	t.Parallel()

	repository := &chatRepository{conversation: domain.Conversation{ID: 3, SellerID: 1, BuyerID: 2}}
	service := app.NewChatService(chatListingReader{}, repository, nil, time.Now)
	if err := service.MarkConversationRead(context.Background(), 1, 3, 9); err != nil {
		t.Fatalf("MarkConversationRead() error = %v", err)
	}
	if repository.readUserID != 1 || repository.readMessageID != 9 {
		t.Errorf("read call = user %d message %d", repository.readUserID, repository.readMessageID)
	}
}

type chatListingReader struct {
	listing domain.Listing
	err     error
}

func (r chatListingReader) GetListing(context.Context, int64) (domain.Listing, error) {
	return r.listing, r.err
}

type chatRepository struct {
	conversation   domain.Conversation
	start          app.StartConversationParams
	startCalled    bool
	message        domain.Message
	create         app.CreateMessageParams
	createErr      error
	beforeMessages []domain.Message
	readUserID     int64
	readMessageID  int64
}

func (r *chatRepository) StartConversation(_ context.Context, params app.StartConversationParams) (domain.Conversation, bool, error) {
	r.startCalled = true
	r.start = params
	return r.conversation, true, nil
}
func (r *chatRepository) ListConversations(context.Context, app.ConversationQuery) ([]domain.ConversationSummary, error) {
	return nil, nil
}
func (r *chatRepository) GetConversationForParticipant(context.Context, int64, int64) (domain.Conversation, error) {
	return r.conversation, nil
}
func (r *chatRepository) ListMessagesBefore(context.Context, int64, *int64, int) ([]domain.Message, error) {
	return append([]domain.Message(nil), r.beforeMessages...), nil
}
func (r *chatRepository) ListMessagesAfter(context.Context, int64, int64, int) ([]domain.Message, error) {
	return nil, nil
}
func (r *chatRepository) CreateMessage(_ context.Context, params app.CreateMessageParams) (domain.Message, domain.Conversation, error) {
	r.create = params
	return r.message, r.conversation, r.createErr
}
func (r *chatRepository) AdvanceReadPointer(_ context.Context, _ domain.Conversation, userID, messageID int64) error {
	r.readUserID, r.readMessageID = userID, messageID
	return nil
}

type messagePublisher struct {
	events []domain.MessageCreatedEvent
}

func (p *messagePublisher) PublishMessageCreated(event domain.MessageCreatedEvent) {
	p.events = append(p.events, event)
}

func chatListing(status domain.ListingStatus, moderationStatus domain.ModerationStatus) domain.Listing {
	return domain.Listing{ID: 100, SellerID: 1, Status: status, ModerationStatus: moderationStatus}
}
