package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestChatRoutesUseStrictJSONAndSafeParticipantErrors(t *testing.T) {
	t.Parallel()

	chat := &chatHTTPFake{
		conversation: domain.Conversation{ID: 9, ListingID: 100, SellerID: 1, BuyerID: 42},
		message:      domain.Message{ID: 7, ConversationID: 9, SenderID: 42, Body: "Boleh COD?", CreatedAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)},
		page: app.ConversationPage{Items: []domain.ConversationSummary{{
			Conversation: domain.Conversation{ID: 9, ListingID: 100}, ListingTitle: "Neo 98", ListingStatus: domain.ListingStatusActive,
			Counterpart: domain.PublicUser{ID: 1, Handle: "seller", DisplayName: "Seller"}, UnreadCount: 2,
		}}},
	}
	handler := authenticatedChatHandler(t, chat, nil)
	headers := map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json; charset=utf-8"}

	created := performRequest(handler, http.MethodPost, "/api/v1/listings/100/conversation", "", headers)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":"9"`) || !strings.Contains(created.Body.String(), `"created":true`) {
		t.Fatalf("start = %d %s", created.Code, created.Body.String())
	}
	inbox := performRequest(handler, http.MethodGet, "/api/v1/conversations?limit=20", "", map[string]string{"Cookie": "keebhub_session=valid"})
	if inbox.Code != http.StatusOK || !strings.Contains(inbox.Body.String(), `"unread_count":2`) || !strings.Contains(inbox.Body.String(), `"next_cursor":null`) {
		t.Fatalf("inbox = %d %s", inbox.Code, inbox.Body.String())
	}
	sent := performRequest(handler, http.MethodPost, "/api/v1/conversations/9/messages", `{"body":"Boleh COD?"}`, headers)
	if sent.Code != http.StatusCreated || !strings.Contains(sent.Body.String(), `"sender_id":"42"`) || chat.sentBody != "Boleh COD?" {
		t.Fatalf("send = %d %s body=%q", sent.Code, sent.Body.String(), chat.sentBody)
	}
	read := performRequest(handler, http.MethodPost, "/api/v1/conversations/9/read", `{"last_read_message_id":"7"}`, headers)
	if read.Code != http.StatusNoContent || chat.readMessageID != 7 {
		t.Fatalf("read = %d id=%d", read.Code, chat.readMessageID)
	}

	for _, body := range []string{`{"body":null}`, `{"body":"ok","unknown":true}`, `{"body":"ok"} {}`} {
		response := performRequest(handler, http.MethodPost, "/api/v1/conversations/9/messages", body, headers)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
			t.Errorf("body %s status = %d", body, response.Code)
		}
	}
	query := performRequest(handler, http.MethodGet, "/api/v1/conversations/9/messages?before_id=7&after_id=8", "", map[string]string{"Cookie": "keebhub_session=valid"})
	if query.Code != http.StatusBadRequest {
		t.Errorf("combined message directions = %d %s", query.Code, query.Body.String())
	}
	chat.listMessagesErr = domain.ErrNotFound
	notFound := performRequest(handler, http.MethodGet, "/api/v1/conversations/9/messages", "", map[string]string{"Cookie": "keebhub_session=valid"})
	if notFound.Code != http.StatusNotFound || !strings.Contains(notFound.Body.String(), `"conversation_not_found"`) {
		t.Errorf("participant isolation = %d %s", notFound.Code, notFound.Body.String())
	}
}

func TestEventsStreamUsesSSEFormatAndReleasesSubscription(t *testing.T) {
	t.Parallel()

	source := &eventSourceFake{events: make(chan domain.MessageCreatedEvent, 1), subscribed: make(chan struct{})}
	handler := authenticatedChatHandler(t, &chatHTTPFake{}, source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	request.Header.Set("Cookie", "keebhub_session=valid")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-source.subscribed:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not subscribe")
	}
	source.events <- domain.MessageCreatedEvent{ConversationID: 9, MessageID: 7, SellerID: 1, BuyerID: 42}
	close(source.events)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not exit after subscription closure")
	}
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(response.Body.String(), "id: 7\nevent: message.created\ndata: {\"conversation_id\":\"9\",\"message_id\":\"7\"}\n\n") || !source.unsubscribed {
		t.Errorf("SSE response = %d %q, unsubscribed=%v", response.Code, response.Body.String(), source.unsubscribed)
	}
}

type chatHTTPFake struct {
	conversation    domain.Conversation
	message         domain.Message
	page            app.ConversationPage
	listMessagesErr error
	sentBody        string
	readMessageID   int64
}

func (f *chatHTTPFake) StartConversation(context.Context, domain.User, int64) (domain.Conversation, bool, error) {
	return f.conversation, true, nil
}
func (f *chatHTTPFake) ListConversations(context.Context, int64, app.ConversationOptions) (app.ConversationPage, error) {
	return f.page, nil
}
func (f *chatHTTPFake) ListMessages(_ context.Context, _ int64, _ int64, options app.MessageOptions) ([]domain.Message, error) {
	if f.listMessagesErr != nil {
		return nil, f.listMessagesErr
	}
	if options.BeforeID != nil && options.AfterID != nil {
		return nil, domain.NewQueryError(map[string]string{"before_id": "cannot be combined with after_id"})
	}
	return []domain.Message{f.message}, nil
}
func (f *chatHTTPFake) SendMessage(_ context.Context, _ domain.User, _ int64, body string) (domain.Message, error) {
	f.sentBody = body
	return f.message, nil
}
func (f *chatHTTPFake) MarkConversationRead(_ context.Context, _ int64, _ int64, messageID int64) error {
	f.readMessageID = messageID
	return nil
}

type eventSourceFake struct {
	events       chan domain.MessageCreatedEvent
	subscribed   chan struct{}
	mu           sync.Mutex
	unsubscribed bool
	err          error
}

func (s *eventSourceFake) SubscribeStream(int64) (<-chan domain.MessageCreatedEvent, func(), error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	close(s.subscribed)
	return s.events, func() {
		s.mu.Lock()
		s.unsubscribed = true
		s.mu.Unlock()
	}, nil
}

func authenticatedChatHandler(t *testing.T, chat httpapi.ChatService, events httpapi.EventSubscriptionSource) http.Handler {
	t.Helper()
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL: "http://localhost:8080",
		Auth: &fakeAuthenticator{authenticateUser: domain.User{
			ID: 42, Handle: "buyer", DiscordUsername: "buyer", DisplayName: "Buyer", Status: domain.UserStatusActive,
		}},
		Chat: chat, Events: events, SessionCookieName: "keebhub_session", SSEKeepaliveInterval: time.Hour,
	})
}

var _ httpapi.ChatService = (*chatHTTPFake)(nil)
var _ httpapi.EventSubscriptionSource = (*eventSourceFake)(nil)
