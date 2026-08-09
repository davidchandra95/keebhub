package sse_test

import (
	"sync"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/sse"
	"github.com/davidchandra95/keebhub/internal/domain"
	"go.uber.org/zap"
)

func TestBrokerDeliversToBothParticipantsAndTabs(t *testing.T) {
	t.Parallel()

	broker := sse.NewBroker(zap.NewNop())
	sellerOne, err := broker.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	sellerTwo, err := broker.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := broker.Subscribe(2)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := broker.Subscribe(3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sellerOne.Unsubscribe)
	t.Cleanup(sellerTwo.Unsubscribe)
	t.Cleanup(buyer.Unsubscribe)
	t.Cleanup(unrelated.Unsubscribe)

	event := domain.MessageCreatedEvent{ConversationID: 9, MessageID: 7, SellerID: 1, BuyerID: 2}
	broker.PublishMessageCreated(event)
	for _, subscription := range []sse.Subscription{sellerOne, sellerTwo, buyer} {
		select {
		case got := <-subscription.Events:
			if got != event {
				t.Errorf("event = %+v, want %+v", got, event)
			}
		case <-time.After(time.Second):
			t.Fatal("event was not delivered")
		}
	}
	select {
	case event := <-unrelated.Events:
		t.Fatalf("unrelated subscriber received %+v", event)
	default:
	}
}

func TestBrokerDropsFullSubscriptionAndCloseIsRaceSafe(t *testing.T) {
	t.Parallel()

	broker := sse.NewBroker(zap.NewNop())
	subscription, err := broker.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	for messageID := int64(1); messageID <= 17; messageID++ {
		broker.PublishMessageCreated(domain.MessageCreatedEvent{ConversationID: 9, MessageID: messageID, SellerID: 1, BuyerID: 2})
	}
	for range subscription.Events {
	}

	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = broker.Subscribe(1)
			broker.PublishMessageCreated(domain.MessageCreatedEvent{ConversationID: 9, MessageID: 99, SellerID: 1, BuyerID: 2})
			subscription.Unsubscribe()
		}()
	}
	broker.Close()
	wait.Wait()
	if _, err := broker.Subscribe(1); err == nil {
		t.Fatal("Subscribe() after Close() error = nil")
	}
}
