package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const chatCursorVersion = 1

// ChatListingReader is the listing information required to start a conversation.
type ChatListingReader interface {
	GetListing(ctx context.Context, listingID int64) (domain.Listing, error)
}

// ChatRepository owns durable chat reads and mutations.
type ChatRepository interface {
	StartConversation(ctx context.Context, params StartConversationParams) (domain.Conversation, bool, error)
	ListConversations(ctx context.Context, query ConversationQuery) ([]domain.ConversationSummary, error)
	GetConversationForParticipant(ctx context.Context, conversationID, userID int64) (domain.Conversation, error)
	ListMessagesBefore(ctx context.Context, conversationID int64, beforeID *int64, limit int) ([]domain.Message, error)
	ListMessagesAfter(ctx context.Context, conversationID, afterID int64, limit int) ([]domain.Message, error)
	CreateMessage(ctx context.Context, params CreateMessageParams) (domain.Message, domain.Conversation, error)
	AdvanceReadPointer(ctx context.Context, conversation domain.Conversation, userID, messageID int64) error
}

// MessageCreatedPublisher publishes a non-durable notification after persistence succeeds.
type MessageCreatedPublisher interface {
	PublishMessageCreated(domain.MessageCreatedEvent)
}

type ChatService struct {
	listings   ChatListingReader
	repository ChatRepository
	publisher  MessageCreatedPublisher
	now        func() time.Time
}

func NewChatService(listings ChatListingReader, repository ChatRepository, publisher MessageCreatedPublisher, now func() time.Time) *ChatService {
	if now == nil {
		now = time.Now
	}
	return &ChatService{listings: listings, repository: repository, publisher: publisher, now: now}
}

type StartConversationParams struct {
	ListingID int64
	SellerID  int64
	BuyerID   int64
	CreatedAt time.Time
}

type ConversationQuery struct {
	UserID           int64
	CursorActivityAt *time.Time
	CursorID         *int64
	Limit            int
}

type ConversationOptions struct {
	Cursor string
	Limit  int
}

type ConversationPage struct {
	Items      []domain.ConversationSummary
	NextCursor *string
}

type MessageOptions struct {
	BeforeID *int64
	AfterID  *int64
	Limit    int
}

type CreateMessageParams struct {
	ConversationID int64
	SenderID       int64
	Body           string
	CreatedAt      time.Time
}

func (s *ChatService) StartConversation(ctx context.Context, user domain.User, listingID int64) (domain.Conversation, bool, error) {
	if err := s.available(); err != nil {
		return domain.Conversation{}, false, err
	}
	if user.Status == domain.UserStatusDisabled {
		return domain.Conversation{}, false, domain.ErrUserDisabled
	}
	listing, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return domain.Conversation{}, false, err
	}
	if listing.ModerationStatus != domain.ModerationStatusVisible || (listing.Status != domain.ListingStatusActive && listing.Status != domain.ListingStatusReserved) {
		return domain.Conversation{}, false, domain.ErrListingUnavailable
	}
	if listing.SellerID == user.ID {
		return domain.Conversation{}, false, domain.ErrSelfConversation
	}
	conversation, created, err := s.repository.StartConversation(ctx, StartConversationParams{
		ListingID: listing.ID, SellerID: listing.SellerID, BuyerID: user.ID, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return domain.Conversation{}, false, err
	}
	return conversation, created, nil
}

func (s *ChatService) ListConversations(ctx context.Context, userID int64, options ConversationOptions) (ConversationPage, error) {
	if err := s.available(); err != nil {
		return ConversationPage{}, err
	}
	limit, err := domain.ValidateChatPageLimit(options.Limit)
	if err != nil {
		return ConversationPage{}, err
	}
	cursor, err := decodeChatCursor(options.Cursor)
	if err != nil {
		return ConversationPage{}, err
	}
	items, err := s.repository.ListConversations(ctx, ConversationQuery{
		UserID: userID, CursorActivityAt: cursor.activityAt, CursorID: cursor.id, Limit: limit + 1,
	})
	if err != nil {
		return ConversationPage{}, err
	}
	page := ConversationPage{Items: items}
	if len(page.Items) <= limit {
		return page, nil
	}
	page.Items = page.Items[:limit]
	last := page.Items[len(page.Items)-1]
	activityAt := last.Conversation.CreatedAt
	if last.Conversation.LastMessageAt != nil {
		activityAt = *last.Conversation.LastMessageAt
	}
	encoded, err := encodeChatCursor(activityAt, last.Conversation.ID)
	if err != nil {
		return ConversationPage{}, err
	}
	page.NextCursor = &encoded
	return page, nil
}

func (s *ChatService) ListMessages(ctx context.Context, userID, conversationID int64, options MessageOptions) ([]domain.Message, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	limit, err := domain.ValidateChatPageLimit(options.Limit)
	if err != nil {
		return nil, err
	}
	if options.BeforeID != nil && options.AfterID != nil {
		return nil, domain.NewQueryError(map[string]string{"before_id": "cannot be combined with after_id", "after_id": "cannot be combined with before_id"})
	}
	if options.BeforeID != nil && *options.BeforeID < 1 {
		return nil, domain.NewQueryError(map[string]string{"before_id": "must be a positive integer"})
	}
	if options.AfterID != nil && *options.AfterID < 1 {
		return nil, domain.NewQueryError(map[string]string{"after_id": "must be a positive integer"})
	}
	if _, err := s.repository.GetConversationForParticipant(ctx, conversationID, userID); err != nil {
		return nil, err
	}
	if options.AfterID != nil {
		return s.repository.ListMessagesAfter(ctx, conversationID, *options.AfterID, limit)
	}
	items, err := s.repository.ListMessagesBefore(ctx, conversationID, options.BeforeID, limit)
	if err != nil {
		return nil, err
	}
	reverseMessages(items)
	return items, nil
}

func (s *ChatService) SendMessage(ctx context.Context, user domain.User, conversationID int64, body string) (domain.Message, error) {
	if err := s.available(); err != nil {
		return domain.Message{}, err
	}
	if user.Status == domain.UserStatusDisabled {
		return domain.Message{}, domain.ErrUserDisabled
	}
	body = domain.NormalizeMessageBody(body)
	if err := domain.ValidateMessageBody(body); err != nil {
		return domain.Message{}, err
	}
	message, conversation, err := s.repository.CreateMessage(ctx, CreateMessageParams{
		ConversationID: conversationID, SenderID: user.ID, Body: body, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return domain.Message{}, err
	}
	if s.publisher != nil {
		s.publisher.PublishMessageCreated(domain.MessageCreatedEvent{
			ConversationID: conversation.ID, MessageID: message.ID, SellerID: conversation.SellerID, BuyerID: conversation.BuyerID,
		})
	}
	return message, nil
}

func (s *ChatService) MarkConversationRead(ctx context.Context, userID, conversationID, messageID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	if messageID < 1 {
		return domain.NewValidationError(map[string]string{"last_read_message_id": "must be a positive integer"})
	}
	conversation, err := s.repository.GetConversationForParticipant(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if err := s.repository.AdvanceReadPointer(ctx, conversation, userID, messageID); err != nil {
		return err
	}
	return nil
}

func (s *ChatService) available() error {
	if s.listings == nil || s.repository == nil {
		return errors.New("chat dependencies are not configured")
	}
	return nil
}

type chatCursor struct {
	Version    int    `json:"v"`
	ActivityAt string `json:"a"`
	ID         int64  `json:"id"`

	activityAt *time.Time
	id         *int64
}

func encodeChatCursor(activityAt time.Time, id int64) (string, error) {
	payload, err := json.Marshal(chatCursor{Version: chatCursorVersion, ActivityAt: activityAt.UTC().Format(time.RFC3339Nano), ID: id})
	if err != nil {
		return "", fmt.Errorf("encode conversation cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeChatCursor(value string) (chatCursor, error) {
	if value == "" {
		return chatCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return chatCursor{}, invalidChatCursor()
	}
	var cursor chatCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != chatCursorVersion || cursor.ID < 1 {
		return chatCursor{}, invalidChatCursor()
	}
	activityAt, err := time.Parse(time.RFC3339Nano, cursor.ActivityAt)
	if err != nil {
		return chatCursor{}, invalidChatCursor()
	}
	cursor.activityAt = &activityAt
	cursor.id = &cursor.ID
	return cursor, nil
}

func invalidChatCursor() error {
	return domain.NewQueryError(map[string]string{"cursor": "is invalid"})
}

func reverseMessages(items []domain.Message) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
