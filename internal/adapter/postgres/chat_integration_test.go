package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestChatStoreConversationMessageAndReadLifecycle(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000101", "chat-seller")
	buyerID := insertCatalogUser(t, ctx, database, "700000000000000102", "chat-buyer")
	thirdUserID := insertCatalogUser(t, ctx, database, "700000000000000103", "chat-third")
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	catalogStore := postgresadapter.NewCatalogStore(database.Pool)
	catalog := app.NewCatalogService(catalogStore, catalogStore, func() time.Time { return now })
	listing := createCatalogListing(t, ctx, catalog, sellerID, "Chat listing", 1_000_000, "keyboard")
	chat := app.NewChatService(catalogStore, postgresadapter.NewChatStore(database.Pool), nil, func() time.Time { return now })
	buyer := domain.User{ID: buyerID, Status: domain.UserStatusActive}
	seller := domain.User{ID: sellerID, Status: domain.UserStatusActive}

	conversation, created, err := chat.StartConversation(ctx, buyer, listing.ID)
	if err != nil || !created || conversation.ID == 0 {
		t.Fatalf("StartConversation() = %+v, %v, %v", conversation, created, err)
	}
	sameConversation, created, err := chat.StartConversation(ctx, buyer, listing.ID)
	if err != nil || created || sameConversation.ID != conversation.ID {
		t.Fatalf("repeat StartConversation() = %+v, %v, %v", sameConversation, created, err)
	}

	first, err := chat.SendMessage(ctx, buyer, conversation.ID, "  Boleh COD?  ")
	if err != nil || first.Body != "Boleh COD?" {
		t.Fatalf("first SendMessage() = %+v, %v", first, err)
	}
	sellerInbox, err := chat.ListConversations(ctx, sellerID, app.ConversationOptions{})
	if err != nil || len(sellerInbox.Items) != 1 || sellerInbox.Items[0].UnreadCount != 1 || sellerInbox.Items[0].LastMessage == nil || sellerInbox.Items[0].LastMessage.SenderID != buyerID {
		t.Fatalf("seller inbox = %+v, %v", sellerInbox, err)
	}
	if err := chat.MarkConversationRead(ctx, sellerID, conversation.ID, first.ID); err != nil {
		t.Fatalf("MarkConversationRead() error = %v", err)
	}
	if err := chat.MarkConversationRead(ctx, sellerID, conversation.ID, first.ID); err != nil {
		t.Fatalf("repeat MarkConversationRead() error = %v", err)
	}
	second, err := chat.SendMessage(ctx, seller, conversation.ID, "Bisa, chat dulu ya")
	if err != nil {
		t.Fatalf("second SendMessage() error = %v", err)
	}
	buyerInbox, err := chat.ListConversations(ctx, buyerID, app.ConversationOptions{})
	if err != nil || len(buyerInbox.Items) != 1 || buyerInbox.Items[0].UnreadCount != 1 || buyerInbox.Items[0].Counterpart.ID != sellerID {
		t.Fatalf("buyer inbox = %+v, %v", buyerInbox, err)
	}
	messages, err := chat.ListMessages(ctx, buyerID, conversation.ID, app.MessageOptions{})
	if err != nil || len(messages) != 2 || messages[0].ID != first.ID || messages[1].ID != second.ID {
		t.Fatalf("latest messages = %+v, %v", messages, err)
	}
	after, err := chat.ListMessages(ctx, buyerID, conversation.ID, app.MessageOptions{AfterID: &first.ID})
	if err != nil || len(after) != 1 || after[0].ID != second.ID {
		t.Fatalf("catch-up messages = %+v, %v", after, err)
	}

	otherListing := createCatalogListing(t, ctx, catalog, sellerID, "Second chat listing", 2_000_000, "keyboard")
	otherConversation, _, err := chat.StartConversation(ctx, buyer, otherListing.ID)
	if err != nil {
		t.Fatalf("second conversation: %v", err)
	}
	if err := chat.MarkConversationRead(ctx, buyerID, otherConversation.ID, second.ID); !errors.Is(err, domain.ErrInvalidReadTarget) {
		t.Errorf("cross-conversation read error = %v", err)
	}
	if _, err := chat.ListMessages(ctx, thirdUserID, conversation.ID, app.MessageOptions{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("third-party list error = %v", err)
	}

	if _, err := catalog.ChangeListingStatus(ctx, sellerID, listing.ID, domain.ListingStatusSold); err != nil {
		t.Fatalf("sell listing: %v", err)
	}
	if _, err := chat.SendMessage(ctx, buyer, conversation.ID, "Still available?"); err != nil {
		t.Errorf("existing conversation after sale: %v", err)
	}
	if _, _, err := chat.StartConversation(ctx, domain.User{ID: thirdUserID, Status: domain.UserStatusActive}, listing.ID); !errors.Is(err, domain.ErrListingUnavailable) {
		t.Errorf("new conversation after sale error = %v", err)
	}
}

func TestChatDatabaseConstraints(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000201", "constraint-seller")
	buyerID := insertCatalogUser(t, ctx, database, "700000000000000202", "constraint-buyer")
	categoryID := int64(1)
	var listingID int64
	if err := database.Pool.QueryRow(ctx, `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition) VALUES ($1, $2, 'Constraint listing', 1, 1, 'used') RETURNING id`, sellerID, categoryID).Scan(&listingID); err != nil {
		t.Fatalf("insert listing: %v", err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO conversations (listing_id, seller_id, buyer_id) VALUES ($1, $2, $2)`, listingID, sellerID); err == nil {
		t.Error("self conversation was accepted")
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO messages (conversation_id, sender_id, body) VALUES (999, $1, 'body')`, sellerID); err == nil {
		t.Error("message with a missing conversation was accepted")
	}
	var conversationID int64
	if err := database.Pool.QueryRow(ctx, `INSERT INTO conversations (listing_id, seller_id, buyer_id) VALUES ($1, $2, $3) RETURNING id`, listingID, sellerID, buyerID).Scan(&conversationID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO messages (conversation_id, sender_id, body) VALUES ($1, $2, '')`, conversationID, buyerID); err == nil {
		t.Error("empty message was accepted")
	}
}

func TestChatStoreConcurrentConversationStartReturnsOneCreatedRow(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000301", "concurrent-seller")
	buyerID := insertCatalogUser(t, ctx, database, "700000000000000302", "concurrent-buyer")
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	catalogStore := postgresadapter.NewCatalogStore(database.Pool)
	catalog := app.NewCatalogService(catalogStore, catalogStore, func() time.Time { return now })
	listing := createCatalogListing(t, ctx, catalog, sellerID, "Concurrent listing", 1_000_000, "keyboard")
	chat := app.NewChatService(catalogStore, postgresadapter.NewChatStore(database.Pool), nil, func() time.Time { return now })

	type result struct {
		conversation domain.Conversation
		created      bool
		err          error
	}
	results := make(chan result, 8)
	var wait sync.WaitGroup
	for index := 0; index < cap(results); index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			conversation, created, err := chat.StartConversation(ctx, domain.User{ID: buyerID, Status: domain.UserStatusActive}, listing.ID)
			results <- result{conversation: conversation, created: created, err: err}
		}()
	}
	wait.Wait()
	close(results)

	var conversationID int64
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("StartConversation() error = %v", result.err)
		}
		if conversationID == 0 {
			conversationID = result.conversation.ID
		}
		if result.conversation.ID != conversationID {
			t.Errorf("conversation ID = %d, want %d", result.conversation.ID, conversationID)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Errorf("created responses = %d, want 1", createdCount)
	}
	var storedCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE listing_id = $1 AND seller_id = $2 AND buyer_id = $3`, listing.ID, sellerID, buyerID).Scan(&storedCount); err != nil || storedCount != 1 {
		t.Errorf("stored conversations = %d, error = %v", storedCount, err)
	}
}
