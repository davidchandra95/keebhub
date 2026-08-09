package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const defaultSSEKeepaliveInterval = 20 * time.Second

// ChatService describes chat use cases consumed by the HTTP adapter.
type ChatService interface {
	StartConversation(ctx context.Context, user domain.User, listingID int64) (domain.Conversation, bool, error)
	ListConversations(ctx context.Context, userID int64, options app.ConversationOptions) (app.ConversationPage, error)
	ListMessages(ctx context.Context, userID, conversationID int64, options app.MessageOptions) ([]domain.Message, error)
	SendMessage(ctx context.Context, user domain.User, conversationID int64, body string) (domain.Message, error)
	MarkConversationRead(ctx context.Context, userID, conversationID, messageID int64) error
}

// EventSubscriptionSource provides the authenticated user's best-effort event stream.
type EventSubscriptionSource interface {
	SubscribeStream(userID int64) (<-chan domain.MessageCreatedEvent, func(), error)
}

type chatHandlers struct {
	chat              ChatService
	events            EventSubscriptionSource
	keepaliveInterval time.Duration
	logger            *zap.Logger
}

func newChatHandlers(cfg Config) chatHandlers {
	keepaliveInterval := cfg.SSEKeepaliveInterval
	if keepaliveInterval <= 0 {
		keepaliveInterval = defaultSSEKeepaliveInterval
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return chatHandlers{chat: cfg.Chat, events: cfg.Events, keepaliveInterval: keepaliveInterval, logger: logger}
}

func (h chatHandlers) startConversation(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	listingID, err := listingIDFromPath(c)
	if err != nil {
		return err
	}
	conversation, created, err := h.chat.StartConversation(c.Request().Context(), user, listingID)
	if err != nil {
		return chatError(err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.JSON(status, conversationCreatedResponse{Conversation: conversationIDResponse{ID: idString(conversation.ID)}, Created: created})
}

func (h chatHandlers) listConversations(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	limit, err := optionalQueryInt(c, "limit")
	if err != nil {
		return err
	}
	page, err := h.chat.ListConversations(c.Request().Context(), user.ID, app.ConversationOptions{Cursor: c.QueryParam("cursor"), Limit: limit})
	if err != nil {
		return chatError(err)
	}
	return c.JSON(http.StatusOK, conversationPageResponseFromDomain(page))
}

func (h chatHandlers) listMessages(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	conversationID, err := conversationIDFromPath(c)
	if err != nil {
		return err
	}
	beforeID, err := optionalPositiveQueryID(c, "before_id")
	if err != nil {
		return err
	}
	afterID, err := optionalPositiveQueryID(c, "after_id")
	if err != nil {
		return err
	}
	limit, err := optionalQueryInt(c, "limit")
	if err != nil {
		return err
	}
	items, err := h.chat.ListMessages(c.Request().Context(), user.ID, conversationID, app.MessageOptions{BeforeID: beforeID, AfterID: afterID, Limit: limit})
	if err != nil {
		return chatError(err)
	}
	return c.JSON(http.StatusOK, messagesResponseFromDomain(items))
}

func (h chatHandlers) sendMessage(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	conversationID, err := conversationIDFromPath(c)
	if err != nil {
		return err
	}
	var request createMessageRequest
	if err := decodeListingRequest(c, &request); err != nil {
		return err
	}
	body, err := request.body()
	if err != nil {
		return chatError(err)
	}
	message, err := h.chat.SendMessage(c.Request().Context(), user, conversationID, body)
	if err != nil {
		return chatError(err)
	}
	h.logger.Info("message_created",
		zap.String("request_id", RequestID(c)), zap.Int64("conversation_id", conversationID), zap.Int64("message_id", message.ID), zap.Int64("sender_id", user.ID),
	)
	return c.JSON(http.StatusCreated, messageEnvelope{Message: messageResponseFromDomain(message)})
}

func (h chatHandlers) markConversationRead(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	conversationID, err := conversationIDFromPath(c)
	if err != nil {
		return err
	}
	var request markReadRequest
	if err := decodeListingRequest(c, &request); err != nil {
		return err
	}
	messageID, err := request.messageID()
	if err != nil {
		return chatError(err)
	}
	if err := h.chat.MarkConversationRead(c.Request().Context(), user.ID, conversationID, messageID); err != nil {
		return chatError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h chatHandlers) streamEvents(c *echo.Context) error {
	if h.events == nil {
		return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(errors.New("SSE broker is not configured"))
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	events, unsubscribe, err := h.events.SubscribeStream(user.ID)
	if err != nil {
		return (&Error{Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "The service is temporarily unavailable."}).Wrap(err)
	}
	defer unsubscribe()

	response := c.Response()
	flusher, ok := response.(http.Flusher)
	if !ok {
		return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(errors.New("response writer does not support flushing"))
	}
	response.Header().Set(echo.HeaderContentType, "text/event-stream")
	response.Header().Set(echo.HeaderCacheControl, "no-cache")
	response.Header().Set("X-Accel-Buffering", "no")
	response.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(h.keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case event, open := <-events:
			if !open {
				return nil
			}
			if err := writeMessageCreatedEvent(response, flusher, event); err != nil {
				return nil
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(response, ": keepalive\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

func (h chatHandlers) available() error {
	if h.chat != nil {
		return nil
	}
	return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(errors.New("chat service is not configured"))
}

type conversationIDResponse struct {
	ID string `json:"id"`
}

type conversationCreatedResponse struct {
	Conversation conversationIDResponse `json:"conversation"`
	Created      bool                   `json:"created"`
}

type conversationListingResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type conversationSummaryResponse struct {
	ID          string                      `json:"id"`
	Listing     conversationListingResponse `json:"listing"`
	Counterpart publicUserResponse          `json:"counterpart"`
	LastMessage *messageResponse            `json:"last_message"`
	UnreadCount int64                       `json:"unread_count"`
}

type conversationPageResponse struct {
	Items      []conversationSummaryResponse `json:"items"`
	NextCursor *string                       `json:"next_cursor"`
}

type messageResponse struct {
	ID        string `json:"id"`
	SenderID  string `json:"sender_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type messageEnvelope struct {
	Message messageResponse `json:"message"`
}

type messagesResponse struct {
	Items []messageResponse `json:"items"`
}

func conversationPageResponseFromDomain(page app.ConversationPage) conversationPageResponse {
	items := make([]conversationSummaryResponse, 0, len(page.Items))
	for _, item := range page.Items {
		response := conversationSummaryResponse{
			ID:          idString(item.Conversation.ID),
			Listing:     conversationListingResponse{ID: idString(item.Conversation.ListingID), Title: item.ListingTitle, Status: string(item.ListingStatus)},
			Counterpart: publicUserResponse{ID: idString(item.Counterpart.ID), Handle: item.Counterpart.Handle, DisplayName: item.Counterpart.DisplayName, AvatarURL: item.Counterpart.AvatarURL},
			UnreadCount: item.UnreadCount,
		}
		if item.LastMessage != nil {
			message := messageResponseFromDomain(*item.LastMessage)
			response.LastMessage = &message
		}
		items = append(items, response)
	}
	return conversationPageResponse{Items: items, NextCursor: page.NextCursor}
}

func messageResponseFromDomain(message domain.Message) messageResponse {
	return messageResponse{ID: idString(message.ID), SenderID: idString(message.SenderID), Body: message.Body, CreatedAt: formatTimestamp(message.CreatedAt)}
}

func messagesResponseFromDomain(messages []domain.Message) messagesResponse {
	items := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		items = append(items, messageResponseFromDomain(message))
	}
	return messagesResponse{Items: items}
}

func idString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func conversationIDFromPath(c *echo.Context) (int64, error) {
	value, err := strconv.ParseInt(c.Param("conversation_id"), 10, 64)
	if err != nil || value < 1 {
		return 0, conversationNotFoundError()
	}
	return value, nil
}

func optionalPositiveQueryID(c *echo.Context, name string) (*int64, error) {
	values, present := c.QueryParams()[name]
	if !present || len(values) == 0 {
		return nil, nil
	}
	value, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || value < 1 {
		return nil, malformedQueryError(name, "must be a positive integer")
	}
	return &value, nil
}

type createMessageRequest struct {
	Body jsonField[string] `json:"body"`
}

func (r createMessageRequest) body() (string, error) {
	fields := requiredFieldErrors(map[string]jsonFieldMarker{"body": r.Body})
	appendNullFieldErrors(fields, map[string]jsonFieldMarker{"body": r.Body})
	if len(fields) > 0 {
		return "", domain.NewValidationError(fields)
	}
	return r.Body.value, nil
}

type markReadRequest struct {
	LastReadMessageID jsonField[string] `json:"last_read_message_id"`
}

func (r markReadRequest) messageID() (int64, error) {
	fields := requiredFieldErrors(map[string]jsonFieldMarker{"last_read_message_id": r.LastReadMessageID})
	appendNullFieldErrors(fields, map[string]jsonFieldMarker{"last_read_message_id": r.LastReadMessageID})
	if len(fields) > 0 {
		return 0, domain.NewValidationError(fields)
	}
	messageID, err := strconv.ParseInt(r.LastReadMessageID.value, 10, 64)
	if err != nil || messageID < 1 {
		return 0, domain.NewValidationError(map[string]string{"last_read_message_id": "must be a positive integer"})
	}
	return messageID, nil
}

func conversationNotFoundError() error {
	return &Error{Status: http.StatusNotFound, Code: "conversation_not_found", Message: "Conversation was not found."}
}

func chatError(err error) error {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Request validation failed.", Fields: validation.Fields, Err: err}
	}
	var query *domain.QueryError
	if errors.As(err, &query) {
		return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed.", Fields: query.Fields, Err: err}
	}
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return conversationNotFoundError()
	case errors.Is(err, domain.ErrSelfConversation), errors.Is(err, domain.ErrListingUnavailable):
		return &Error{Status: http.StatusConflict, Code: "conversation_unavailable", Message: "A conversation cannot be started for this listing.", Err: err}
	case errors.Is(err, domain.ErrInvalidReadTarget):
		return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Request validation failed.", Fields: map[string]string{"last_read_message_id": "must belong to this conversation"}, Err: err}
	case errors.Is(err, domain.ErrUserDisabled):
		return &Error{Status: http.StatusForbidden, Code: "account_disabled", Message: "This account is disabled.", Err: err}
	default:
		return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(err)
	}
}

type messageCreatedEventData struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

func writeMessageCreatedEvent(response http.ResponseWriter, flusher http.Flusher, event domain.MessageCreatedEvent) error {
	payload, err := json.Marshal(messageCreatedEventData{ConversationID: idString(event.ConversationID), MessageID: idString(event.MessageID)})
	if err != nil {
		return fmt.Errorf("marshal message event: %w", err)
	}
	if _, err := fmt.Fprintf(response, "id: %d\nevent: message.created\ndata: %s\n\n", event.MessageID, payload); err != nil {
		return fmt.Errorf("write message event: %w", err)
	}
	flusher.Flush()
	return nil
}
