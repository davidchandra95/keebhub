package domain_test

import (
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestNormalizeHandle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		want     string
	}{
		{name: "Discord punctuation", username: "Gunawan.Keyboard", want: "gunawan-keyboard"},
		{name: "separator runs", username: "--Key__Board..", want: "key-board"},
		{name: "short", username: "ab", want: "user-ab"},
		{name: "no ASCII", username: "鍵盤", want: "user"},
		{name: "maximum length", username: strings.Repeat("a", 50), want: strings.Repeat("a", 40)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.NormalizeHandle(tt.username); got != tt.want {
				t.Errorf("NormalizeHandle(%q) = %q, want %q", tt.username, got, tt.want)
			}
		})
	}
}

func TestHandleCandidatePreservesMaximumLength(t *testing.T) {
	t.Parallel()

	base := strings.Repeat("a", 40)
	if got := domain.HandleCandidate(base, 1); got != base {
		t.Errorf("first candidate = %q", got)
	}
	if got := domain.HandleCandidate(base, 12); len(got) != 40 || !strings.HasSuffix(got, "-12") {
		t.Errorf("collision candidate = %q, length %d", got, len(got))
	}
}
