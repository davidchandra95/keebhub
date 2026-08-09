// Package sse implements the process-local best-effort event broker.
package sse

import (
	"errors"
	"sync"

	"github.com/davidchandra95/keebhub/internal/domain"
	"go.uber.org/zap"
)

const subscriptionBufferSize = 16

var ErrBrokerClosed = errors.New("SSE broker is closed")

// Subscription is one browser stream for an authenticated user.
type Subscription struct {
	Events      <-chan domain.MessageCreatedEvent
	unsubscribe func()
}

func (s Subscription) Unsubscribe() {
	if s.unsubscribe != nil {
		s.unsubscribe()
	}
}

type subscriber struct {
	userID int64
	events chan domain.MessageCreatedEvent
}

// Broker safely supports many browser tabs per user in one server process.
type Broker struct {
	mu          sync.Mutex
	closed      bool
	subscribers map[int64]map[*subscriber]struct{}
	logger      *zap.Logger
}

func NewBroker(logger *zap.Logger) *Broker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Broker{subscribers: make(map[int64]map[*subscriber]struct{}), logger: logger}
}

func (b *Broker) Subscribe(userID int64) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Subscription{}, ErrBrokerClosed
	}
	subscription := &subscriber{userID: userID, events: make(chan domain.MessageCreatedEvent, subscriptionBufferSize)}
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = make(map[*subscriber]struct{})
	}
	b.subscribers[userID][subscription] = struct{}{}
	b.logger.Info("sse_connected", zap.Int64("user_id", userID))
	return Subscription{
		Events: subscription.events,
		unsubscribe: func() {
			b.unsubscribe(subscription)
		},
	}, nil
}

// SubscribeStream adapts a subscription for the HTTP SSE handler.
func (b *Broker) SubscribeStream(userID int64) (<-chan domain.MessageCreatedEvent, func(), error) {
	subscription, err := b.Subscribe(userID)
	if err != nil {
		return nil, nil, err
	}
	return subscription.Events, subscription.Unsubscribe, nil
}

// PublishMessageCreated cannot block a committed message response.
func (b *Broker) PublishMessageCreated(event domain.MessageCreatedEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.publishToUserLocked(event.SellerID, event)
	if event.BuyerID != event.SellerID {
		b.publishToUserLocked(event.BuyerID, event)
	}
}

func (b *Broker) publishToUserLocked(userID int64, event domain.MessageCreatedEvent) {
	for subscription := range b.subscribers[userID] {
		select {
		case subscription.events <- event:
		default:
			b.closeSubscriptionLocked(subscription)
			b.logger.Warn("sse_publish_dropped",
				zap.Int64("user_id", userID),
				zap.Int64("conversation_id", event.ConversationID),
				zap.Int64("message_id", event.MessageID),
			)
		}
	}
}

func (b *Broker) unsubscribe(subscription *subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeSubscriptionLocked(subscription) {
		b.logger.Info("sse_disconnected", zap.Int64("user_id", subscription.userID))
	}
}

func (b *Broker) closeSubscriptionLocked(subscription *subscriber) bool {
	userSubscriptions, exists := b.subscribers[subscription.userID]
	if !exists {
		return false
	}
	if _, exists := userSubscriptions[subscription]; !exists {
		return false
	}
	delete(userSubscriptions, subscription)
	if len(userSubscriptions) == 0 {
		delete(b.subscribers, subscription.userID)
	}
	close(subscription.events)
	return true
}

// Close unblocks every active stream and rejects later subscriptions.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, userSubscriptions := range b.subscribers {
		for subscription := range userSubscriptions {
			close(subscription.events)
		}
	}
	b.subscribers = make(map[int64]map[*subscriber]struct{})
}

var _ interface {
	PublishMessageCreated(domain.MessageCreatedEvent)
} = (*Broker)(nil)
