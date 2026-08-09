package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	generateddb "github.com/davidchandra95/keebhub/internal/generated/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChatStore is the PostgreSQL implementation of the chat repository.
type ChatStore struct {
	pool *pgxpool.Pool
}

func NewChatStore(pool *pgxpool.Pool) *ChatStore {
	return &ChatStore{pool: pool}
}

func (s *ChatStore) StartConversation(ctx context.Context, params app.StartConversationParams) (domain.Conversation, bool, error) {
	row, err := generateddb.New(s.pool).StartConversation(ctx, generateddb.StartConversationParams{
		ListingID: params.ListingID, SellerID: params.SellerID, BuyerID: params.BuyerID, CreatedAt: timestamp(params.CreatedAt),
	})
	if err != nil {
		return domain.Conversation{}, false, fmt.Errorf("start conversation: %w", err)
	}
	return conversationFromGenerated(row.ID, row.ListingID, row.SellerID, row.BuyerID, row.SellerLastReadMessageID, row.BuyerLastReadMessageID, row.CreatedAt, row.LastMessageAt), row.Created, nil
}

func (s *ChatStore) ListConversations(ctx context.Context, query app.ConversationQuery) ([]domain.ConversationSummary, error) {
	rows, err := generateddb.New(s.pool).ListConversationsForUser(ctx, generateddb.ListConversationsForUserParams{
		UserID: query.UserID, CursorActivityAt: nullableTimestamp(query.CursorActivityAt), CursorID: query.CursorID, PageLimit: int32(query.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	items := make([]domain.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		conversation := domain.Conversation{
			ID: row.ID, ListingID: row.ListingID, SellerID: row.SellerID, BuyerID: row.BuyerID,
			CreatedAt: row.ConversationCreatedAt.Time,
		}
		if row.ConversationLastMessageAt.Valid {
			lastMessageAt := row.ConversationLastMessageAt.Time
			conversation.LastMessageAt = &lastMessageAt
		}
		summary := domain.ConversationSummary{
			Conversation: conversation,
			ListingTitle: row.ListingTitle, ListingStatus: domain.ListingStatus(row.ListingStatus),
			Counterpart: domain.PublicUser{ID: row.CounterpartID, Handle: row.CounterpartHandle, DisplayName: row.CounterpartDisplayName, AvatarURL: row.CounterpartAvatarUrl},
			UnreadCount: row.UnreadCount,
		}
		if row.LastMessageID != 0 {
			summary.LastMessage = &domain.Message{
				ID: row.LastMessageID, ConversationID: row.ID, SenderID: row.LastMessageSenderID,
				Body: row.LastMessageBody, CreatedAt: row.LastMessageCreatedAt.Time,
			}
		}
		items = append(items, summary)
	}
	return items, nil
}

func (s *ChatStore) GetConversationForParticipant(ctx context.Context, conversationID, userID int64) (domain.Conversation, error) {
	row, err := generateddb.New(s.pool).GetConversationForParticipant(ctx, generateddb.GetConversationForParticipantParams{
		ConversationID: conversationID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("get participant conversation: %w", err)
	}
	return conversationFromGenerated(row.ID, row.ListingID, row.SellerID, row.BuyerID, row.SellerLastReadMessageID, row.BuyerLastReadMessageID, row.CreatedAt, row.LastMessageAt), nil
}

func (s *ChatStore) ListMessagesBefore(ctx context.Context, conversationID int64, beforeID *int64, limit int) ([]domain.Message, error) {
	rows, err := generateddb.New(s.pool).ListMessagesBefore(ctx, generateddb.ListMessagesBeforeParams{
		ConversationID: conversationID, BeforeID: beforeID, PageLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages before: %w", err)
	}
	return messagesFromGenerated(rows), nil
}

func (s *ChatStore) ListMessagesAfter(ctx context.Context, conversationID, afterID int64, limit int) ([]domain.Message, error) {
	rows, err := generateddb.New(s.pool).ListMessagesAfter(ctx, generateddb.ListMessagesAfterParams{
		ConversationID: conversationID, AfterID: afterID, PageLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages after: %w", err)
	}
	return messagesFromGenerated(rows), nil
}

func (s *ChatStore) CreateMessage(ctx context.Context, params app.CreateMessageParams) (domain.Message, domain.Conversation, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Message{}, domain.Conversation{}, fmt.Errorf("begin message transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	queries := generateddb.New(tx)
	conversationRow, err := queries.GetConversationForParticipant(ctx, generateddb.GetConversationForParticipantParams{
		ConversationID: params.ConversationID, UserID: params.SenderID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, domain.Conversation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Message{}, domain.Conversation{}, fmt.Errorf("get message conversation: %w", err)
	}
	messageRow, err := queries.InsertMessage(ctx, generateddb.InsertMessageParams{
		ConversationID: params.ConversationID, SenderID: params.SenderID, Body: params.Body, CreatedAt: timestamp(params.CreatedAt),
	})
	if err != nil {
		return domain.Message{}, domain.Conversation{}, fmt.Errorf("insert message: %w", err)
	}
	if err := queries.UpdateConversationLastMessageAt(ctx, generateddb.UpdateConversationLastMessageAtParams{
		ConversationID: params.ConversationID, LastMessageAt: timestamp(params.CreatedAt),
	}); err != nil {
		return domain.Message{}, domain.Conversation{}, fmt.Errorf("update conversation activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Message{}, domain.Conversation{}, fmt.Errorf("commit message transaction: %w", err)
	}
	conversation := conversationFromGenerated(conversationRow.ID, conversationRow.ListingID, conversationRow.SellerID, conversationRow.BuyerID, conversationRow.SellerLastReadMessageID, conversationRow.BuyerLastReadMessageID, conversationRow.CreatedAt, conversationRow.LastMessageAt)
	return messageFromGenerated(messageRow), conversation, nil
}

func (s *ChatStore) AdvanceReadPointer(ctx context.Context, conversation domain.Conversation, userID, messageID int64) error {
	queries := generateddb.New(s.pool)
	if _, err := queries.GetMessageInConversation(ctx, generateddb.GetMessageInConversationParams{MessageID: messageID, ConversationID: conversation.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrInvalidReadTarget
		}
		return fmt.Errorf("validate read target: %w", err)
	}
	messageIDPointer := &messageID
	if conversation.IsSeller(userID) {
		err := queries.AdvanceSellerReadPointer(ctx, generateddb.AdvanceSellerReadPointerParams{
			MessageID: messageIDPointer, ConversationID: conversation.ID, UserID: userID,
		})
		if err != nil {
			return fmt.Errorf("advance seller read pointer: %w", err)
		}
		return nil
	}
	err := queries.AdvanceBuyerReadPointer(ctx, generateddb.AdvanceBuyerReadPointerParams{
		MessageID: messageIDPointer, ConversationID: conversation.ID, UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("advance buyer read pointer: %w", err)
	}
	return nil
}

func conversationFromGenerated(id, listingID, sellerID, buyerID int64, sellerRead, buyerRead *int64, createdAt, lastMessageAt pgtype.Timestamptz) domain.Conversation {
	conversation := domain.Conversation{
		ID: id, ListingID: listingID, SellerID: sellerID, BuyerID: buyerID,
		SellerLastReadMessageID: sellerRead, BuyerLastReadMessageID: buyerRead, CreatedAt: createdAt.Time,
	}
	if lastMessageAt.Valid {
		value := lastMessageAt.Time
		conversation.LastMessageAt = &value
	}
	return conversation
}

func messageFromGenerated(message generateddb.Message) domain.Message {
	return domain.Message{ID: message.ID, ConversationID: message.ConversationID, SenderID: message.SenderID, Body: message.Body, CreatedAt: message.CreatedAt.Time}
}

func messagesFromGenerated(rows []generateddb.Message) []domain.Message {
	items := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		items = append(items, messageFromGenerated(row))
	}
	return items
}

var _ app.ChatRepository = (*ChatStore)(nil)
