package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestListingStatusTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from    domain.ListingStatus
		to      domain.ListingStatus
		allowed bool
	}{
		{domain.StatusActive, domain.StatusActive, true},
		{domain.StatusActive, domain.StatusReserved, true},
		{domain.StatusActive, domain.StatusSold, true},
		{domain.StatusActive, domain.StatusArchived, true},
		{domain.StatusReserved, domain.StatusActive, true},
		{domain.StatusReserved, domain.StatusReserved, true},
		{domain.StatusReserved, domain.StatusSold, true},
		{domain.StatusReserved, domain.StatusArchived, true},
		{domain.StatusSold, domain.StatusActive, true},
		{domain.StatusSold, domain.StatusSold, true},
		{domain.StatusSold, domain.StatusArchived, true},
		{domain.StatusArchived, domain.StatusActive, true},
		{domain.StatusArchived, domain.StatusArchived, true},
		{domain.StatusArchived, domain.StatusReserved, false},
		{domain.StatusArchived, domain.StatusSold, false},
		{domain.StatusSold, domain.StatusReserved, false},
		{domain.StatusActive, domain.ListingStatus("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"_to_"+string(tt.to), func(t *testing.T) {
			err := domain.CanTransitionListingStatus(tt.from, tt.to)
			if tt.allowed && err != nil {
				t.Fatalf("transition error = %v", err)
			}
			if !tt.allowed && err == nil {
				t.Fatal("transition unexpectedly allowed")
			}
			if !tt.allowed && tt.from != domain.StatusActive && string(tt.to) != "unknown" && !errors.Is(err, domain.ErrConflict) {
				t.Errorf("error = %v, want conflict", err)
			}
		})
	}
}

func TestListingUnicodeBoundariesAndNormalization(t *testing.T) {
	t.Parallel()

	if got := domain.NormalizeListingTitle("  Neo 98  "); got != "Neo 98" {
		t.Errorf("normalized title = %q", got)
	}
	if err := domain.ValidateListingTitle(strings.Repeat("鍵", domain.MaximumListingTitleLength)); err != nil {
		t.Fatalf("maximum Unicode title rejected: %v", err)
	}
	if err := domain.ValidateListingTitle(strings.Repeat("鍵", domain.MaximumListingTitleLength+1)); err == nil {
		t.Fatal("overlong Unicode title accepted")
	}
	description := "line one\nline two"
	if err := domain.ValidateListingDescription(description); err != nil {
		t.Fatalf("description with line break rejected: %v", err)
	}
	if err := domain.ValidateListingDescription(strings.Repeat("語", domain.MaximumListingDescriptionLength+1)); err == nil {
		t.Fatal("overlong Unicode description accepted")
	}
}

func TestListingValueBoundaries(t *testing.T) {
	t.Parallel()

	for _, value := range []int64{domain.MinimumListingPriceIDR, domain.MaximumListingPriceIDR} {
		if err := domain.ValidateListingPrice(value); err != nil {
			t.Errorf("price %d rejected: %v", value, err)
		}
	}
	for _, value := range []int32{domain.MinimumListingQuantity, domain.MaximumListingQuantity} {
		if err := domain.ValidateListingQuantity(value); err != nil {
			t.Errorf("quantity %d rejected: %v", value, err)
		}
	}
	if err := domain.ValidateListingPrice(0); err == nil {
		t.Error("zero price accepted")
	}
	if err := domain.ValidateListingQuantity(0); err == nil {
		t.Error("zero quantity accepted")
	}
}
