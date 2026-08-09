package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestNormalizeProfileValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		normalize func(*string) (*string, error)
		value     *string
		want      *string
		wantError bool
	}{
		{name: "nil location", normalize: domain.NormalizeProfileLocation},
		{name: "empty location clears", normalize: domain.NormalizeProfileLocation, value: stringPointer(" \t "), want: nil},
		{name: "location trims outer spaces", normalize: domain.NormalizeProfileLocation, value: stringPointer(" Jakarta Barat "), want: stringPointer("Jakarta Barat")},
		{name: "bio preserves internal line breaks", normalize: domain.NormalizeProfileBio, value: stringPointer(" Keyboard enthusiast\nwith vintage switches "), want: stringPointer("Keyboard enthusiast\nwith vintage switches")},
		{name: "location unicode boundary", normalize: domain.NormalizeProfileLocation, value: stringPointer(strings.Repeat("界", domain.MaximumProfileLocationLength)), want: stringPointer(strings.Repeat("界", domain.MaximumProfileLocationLength))},
		{name: "location unicode over boundary", normalize: domain.NormalizeProfileLocation, value: stringPointer(strings.Repeat("界", domain.MaximumProfileLocationLength+1)), wantError: true},
		{name: "bio unicode boundary", normalize: domain.NormalizeProfileBio, value: stringPointer(strings.Repeat("界", domain.MaximumProfileBioLength)), want: stringPointer(strings.Repeat("界", domain.MaximumProfileBioLength))},
		{name: "bio unicode over boundary", normalize: domain.NormalizeProfileBio, value: stringPointer(strings.Repeat("界", domain.MaximumProfileBioLength+1)), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.normalize(tt.value)
			if tt.wantError {
				var validation *domain.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("normalize error = %v, want validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize error = %v", err)
			}
			if !sameStringPointer(got, tt.want) {
				t.Errorf("normalized value = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCanonicalHandle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		value string
		want  bool
	}{
		{value: "gunawan", want: true},
		{value: "gunawan-98", want: true},
		{value: "ab", want: false},
		{value: "Gunawan", want: false},
		{value: "gunawan-", want: false},
		{value: "-gunawan", want: false},
		{value: "gunawan_keyboard", want: false},
		{value: strings.Repeat("a", 41), want: false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			if got := domain.IsCanonicalHandle(tt.value); got != tt.want {
				t.Errorf("IsCanonicalHandle(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }

func sameStringPointer(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
