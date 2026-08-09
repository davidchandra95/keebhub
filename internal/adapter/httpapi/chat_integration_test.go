package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/adapter/sse"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestChatHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	catalogStore := postgresadapter.NewCatalogStore(database.Pool)
	catalog := app.NewCatalogService(catalogStore, catalogStore, func() time.Time { return now })
	broker := sse.NewBroker(zap.NewNop())
	t.Cleanup(broker.Close)
	chat := app.NewChatService(catalogStore, postgresadapter.NewChatStore(database.Pool), broker, func() time.Time { return now })

	sellerHandler := newChatIntegrationHandler(t, database.Pool, integrationOAuth{identity: domain.DiscordIdentity{
		ID: "810000000000000001", Username: "chat.seller", DisplayName: "Chat Seller",
	}}, catalog, chat, broker)
	buyerHandler := newChatIntegrationHandler(t, database.Pool, integrationOAuth{identity: domain.DiscordIdentity{
		ID: "810000000000000002", Username: "chat.buyer", DisplayName: "Chat Buyer",
	}}, catalog, chat, broker)
	thirdHandler := newChatIntegrationHandler(t, database.Pool, integrationOAuth{identity: domain.DiscordIdentity{
		ID: "810000000000000003", Username: "chat.third", DisplayName: "Chat Third",
	}}, catalog, chat, broker)
	sellerCookie := completeIntegrationLogin(t, sellerHandler)
	buyerCookie := completeIntegrationLogin(t, buyerHandler)
	thirdCookie := completeIntegrationLogin(t, thirdHandler)

	jsonHeaders := func(cookie *http.Cookie) map[string]string {
		return map[string]string{
			"Origin": "http://localhost:8080", "Cookie": cookie.Name + "=" + cookie.Value, "Content-Type": "application/json",
		}
	}
	listingResponse := performRequest(sellerHandler, http.MethodPost, "/api/v1/listings", `{"title":"Chat Neo","price_idr":3000000,"category_slug":"keyboard","condition":"used"}`, jsonHeaders(sellerCookie))
	if listingResponse.Code != http.StatusCreated {
		t.Fatalf("create listing = %d %s", listingResponse.Code, listingResponse.Body.String())
	}
	listingID := responseListingID(t, listingResponse.Body.Bytes())

	started := performRequest(buyerHandler, http.MethodPost, "/api/v1/listings/"+listingID+"/conversation", "", jsonHeaders(buyerCookie))
	if started.Code != http.StatusCreated {
		t.Fatalf("start conversation = %d %s", started.Code, started.Body.String())
	}
	conversationID := responseConversationID(t, started.Body.Bytes())
	repeated := performRequest(buyerHandler, http.MethodPost, "/api/v1/listings/"+listingID+"/conversation", "", jsonHeaders(buyerCookie))
	if repeated.Code != http.StatusOK || responseConversationID(t, repeated.Body.Bytes()) != conversationID {
		t.Fatalf("repeat conversation = %d %s", repeated.Code, repeated.Body.String())
	}

	sent := performRequest(buyerHandler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", `{"body":"  Boleh COD?  "}`, jsonHeaders(buyerCookie))
	if sent.Code != http.StatusCreated || !containsJSONField(t, sent.Body.Bytes(), "body", "Boleh COD?") {
		t.Fatalf("send message = %d %s", sent.Code, sent.Body.String())
	}
	messageID := responseMessageID(t, sent.Body.Bytes())
	sellerInbox := performRequest(sellerHandler, http.MethodGet, "/api/v1/conversations", "", map[string]string{"Cookie": sellerCookie.Name + "=" + sellerCookie.Value})
	if sellerInbox.Code != http.StatusOK || !containsJSONField(t, sellerInbox.Body.Bytes(), "unread_count", float64(1)) || !containsJSONField(t, sellerInbox.Body.Bytes(), "sender_id", "2") {
		t.Fatalf("seller inbox = %d %s", sellerInbox.Code, sellerInbox.Body.String())
	}
	read := performRequest(sellerHandler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/read", `{"last_read_message_id":"`+messageID+`"}`, jsonHeaders(sellerCookie))
	if read.Code != http.StatusNoContent {
		t.Fatalf("mark read = %d %s", read.Code, read.Body.String())
	}
	history := performRequest(buyerHandler, http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", "", map[string]string{"Cookie": buyerCookie.Name + "=" + buyerCookie.Value})
	if history.Code != http.StatusOK || !containsJSONField(t, history.Body.Bytes(), "id", messageID) {
		t.Fatalf("history = %d %s", history.Code, history.Body.String())
	}
	thirdHistory := performRequest(thirdHandler, http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", "", map[string]string{"Cookie": thirdCookie.Name + "=" + thirdCookie.Value})
	if thirdHistory.Code != http.StatusNotFound {
		t.Fatalf("third-party history = %d %s", thirdHistory.Code, thirdHistory.Body.String())
	}

	sold := performRequest(sellerHandler, http.MethodPost, "/api/v1/listings/"+listingID+"/status", `{"status":"sold"}`, jsonHeaders(sellerCookie))
	if sold.Code != http.StatusOK {
		t.Fatalf("sell listing = %d %s", sold.Code, sold.Body.String())
	}
	existingChat := performRequest(buyerHandler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", `{"body":"Still available?"}`, jsonHeaders(buyerCookie))
	if existingChat.Code != http.StatusCreated {
		t.Fatalf("message after sale = %d %s", existingChat.Code, existingChat.Body.String())
	}
	newChat := performRequest(thirdHandler, http.MethodPost, "/api/v1/listings/"+listingID+"/conversation", "", jsonHeaders(thirdCookie))
	if newChat.Code != http.StatusConflict {
		t.Errorf("new conversation after sale = %d %s", newChat.Code, newChat.Body.String())
	}
}

func newChatIntegrationHandler(t *testing.T, pool *pgxpool.Pool, oauth integrationOAuth, catalog httpapi.CatalogService, chat httpapi.ChatService, events httpapi.EventSubscriptionSource) http.Handler {
	t.Helper()
	auth := app.NewAuthService(oauth, postgresadapter.NewAuthStore(pool))
	return newHandlerConfig(t, pool, httpapi.Config{
		AppBaseURL: "http://localhost:8080", Auth: auth, Catalog: catalog, Chat: chat, Events: events, SessionCookieName: "keebhub_session",
	})
}

func responseConversationID(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Conversation.ID == "" {
		t.Fatalf("decode conversation ID: %v, %s", err, body)
	}
	return response.Conversation.ID
}

func responseMessageID(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Message.ID == "" {
		t.Fatalf("decode message ID: %v, %s", err, body)
	}
	return response.Message.ID
}
