package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestMessageBodyNormalizationAndValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "outer whitespace", body: " \n  Boleh COD?  \t", want: "Boleh COD?"},
		{name: "internal whitespace", body: "one  two\nthree", want: "one  two\nthree"},
		{name: "empty", body: " \n\t ", wantErr: true},
		{name: "maximum unicode length", body: strings.Repeat("鍵", domain.MaximumMessageBodyLength), want: strings.Repeat("鍵", domain.MaximumMessageBodyLength)},
		{name: "too long unicode", body: strings.Repeat("鍵", domain.MaximumMessageBodyLength+1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.NormalizeMessageBody(tt.body)
			err := domain.ValidateMessageBody(got)
			if tt.wantErr {
				var validation *domain.ValidationError
				if !errors.As(err, &validation) || validation.Fields["body"] == "" {
					t.Fatalf("ValidateMessageBody() error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateMessageBody() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("NormalizeMessageBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateChatPageLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{-1, 101} {
		if _, err := domain.ValidateChatPageLimit(limit); err == nil {
			t.Errorf("ValidateChatPageLimit(%d) error = nil", limit)
		}
	}
	if got, err := domain.ValidateChatPageLimit(0); err != nil || got != domain.DefaultChatPageLimit {
		t.Errorf("default page limit = %d, %v", got, err)
	}
}
