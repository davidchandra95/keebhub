package server_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/sse"
	"github.com/davidchandra95/keebhub/internal/domain"
	platformserver "github.com/davidchandra95/keebhub/internal/platform/server"
	"go.uber.org/zap"
)

func TestRunnerServesAndShutsDownWhenContextEnds(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusNoContent)
		}),
		ReadHeaderTimeout: time.Second,
	}
	runner := platformserver.Runner{
		HTTPServer:      httpServer,
		Logger:          zap.NewNop(),
		ShutdownTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, listener)
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		cancel()
		t.Fatalf("GET running server: %v", err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", response.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+listener.Addr().String(), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err = http.DefaultClient.Do(request)
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("server still accepts requests after shutdown")
	}
}

func TestRunnerReturnsUnexpectedServeError(t *testing.T) {
	t.Parallel()

	listener := &failingListener{err: errors.New("accept failed")}
	runner := platformserver.Runner{
		HTTPServer:      &http.Server{Handler: http.NotFoundHandler()},
		Logger:          zap.NewNop(),
		ShutdownTimeout: time.Second,
	}

	err := runner.Run(context.Background(), listener)
	if !errors.Is(err, listener.err) {
		t.Fatalf("Run() error = %v, want %v", err, listener.err)
	}
}

func TestRunnerPreShutdownReleasesSSEStream(t *testing.T) {
	t.Parallel()

	broker := sse.NewBroker(zap.NewNop())
	streamStarted := make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpServer := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		subscription, err := broker.Subscribe(1)
		if err != nil {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
			return
		}
		defer subscription.Unsubscribe()
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		close(streamStarted)
		for range subscription.Events {
		}
	})}
	runner := platformserver.Runner{
		HTTPServer: httpServer, Logger: zap.NewNop(), PreShutdown: broker.Close, ShutdownTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, listener) }()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close stream response: %v", err)
		}
	}()
	select {
	case <-streamStarted:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() waited for the SSE stream instead of closing it")
	}

	broker.PublishMessageCreated(domain.MessageCreatedEvent{ConversationID: 1, MessageID: 1, SellerID: 1, BuyerID: 2})
}

type failingListener struct {
	err error
}

func (l *failingListener) Accept() (net.Conn, error) { return nil, l.err }
func (l *failingListener) Close() error              { return nil }
func (l *failingListener) Addr() net.Addr            { return testAddress("failed") }

type testAddress string

func (a testAddress) Network() string { return string(a) }
func (a testAddress) String() string  { return string(a) }
