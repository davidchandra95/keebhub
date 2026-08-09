package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumMessageBodyLength = 2000
	DefaultChatPageLimit     = 20
	MaximumChatPageLimit     = 100
)

var (
	ErrListingUnavailable = errors.New("listing unavailable for a new conversation")
	ErrSelfConversation   = errors.New("cannot start a conversation with yourself")
	ErrInvalidReadTarget  = errors.New("read target does not belong to the conversation")
)

// Conversation is the durable listing-scoped relationship between a buyer and seller.
type Conversation struct {
	ID                      int64
	ListingID               int64
	SellerID                int64
	BuyerID                 int64
	SellerLastReadMessageID *int64
	BuyerLastReadMessageID  *int64
	CreatedAt               time.Time
	LastMessageAt           *time.Time
}

func (c Conversation) IsParticipant(userID int64) bool {
	return c.SellerID == userID || c.BuyerID == userID
}

func (c Conversation) IsSeller(userID int64) bool {
	return c.SellerID == userID
}

// Message is an immutable plain-text conversation entry.
type Message struct {
	ID             int64
	ConversationID int64
	SenderID       int64
	Body           string
	CreatedAt      time.Time
}

type ConversationSummary struct {
	Conversation  Conversation
	ListingTitle  string
	ListingStatus ListingStatus
	Counterpart   PublicUser
	LastMessage   *Message
	UnreadCount   int64
}

// MessageCreatedEvent is a best-effort notification emitted after a message commits.
type MessageCreatedEvent struct {
	ConversationID int64
	MessageID      int64
	SellerID       int64
	BuyerID        int64
}

func NormalizeMessageBody(value string) string {
	return strings.TrimSpace(value)
}

func ValidateMessageBody(value string) error {
	fields := map[string]string{}
	if value == "" {
		fields["body"] = "must not be blank"
	}
	if utf8.RuneCountInString(value) > MaximumMessageBodyLength {
		fields["body"] = fmt.Sprintf("must be at most %d characters", MaximumMessageBodyLength)
	}
	return NewValidationError(fields)
}

func ValidateChatPageLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultChatPageLimit, nil
	}
	if limit < 1 || limit > MaximumChatPageLimit {
		return 0, NewQueryError(map[string]string{"limit": fmt.Sprintf("must be between 1 and %d", MaximumChatPageLimit)})
	}
	return limit, nil
}
