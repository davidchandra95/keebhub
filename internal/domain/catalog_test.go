package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestListingStatusTransitions(t *testing.T) {
	t.Parallel()

	statuses := []domain.ListingStatus{
		domain.ListingStatusActive,
		domain.ListingStatusReserved,
		domain.ListingStatusSold,
		domain.ListingStatusArchived,
	}
	want := map[domain.ListingStatus]map[domain.ListingStatus]bool{
		domain.ListingStatusActive:   {domain.ListingStatusActive: true, domain.ListingStatusReserved: true, domain.ListingStatusSold: true, domain.ListingStatusArchived: true},
		domain.ListingStatusReserved: {domain.ListingStatusActive: true, domain.ListingStatusReserved: true, domain.ListingStatusSold: true, domain.ListingStatusArchived: true},
		domain.ListingStatusSold:     {domain.ListingStatusActive: true, domain.ListingStatusSold: true, domain.ListingStatusArchived: true},
		domain.ListingStatusArchived: {domain.ListingStatusActive: true, domain.ListingStatusArchived: true},
	}
	for _, from := range statuses {
		for _, to := range statuses {
			from, to := from, to
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()
				if got := domain.CanTransitionListingStatus(from, to); got != want[from][to] {
					t.Errorf("CanTransitionListingStatus(%q, %q) = %v, want %v", from, to, got, want[from][to])
				}
			})
		}
	}
	if domain.CanTransitionListingStatus("unknown", domain.ListingStatusActive) {
		t.Error("unknown listing status was accepted")
	}
}

func TestListingValidationUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "unicode boundary", value: strings.Repeat("界", domain.MaximumListingTitleLength), valid: true},
		{name: "unicode over boundary", value: strings.Repeat("界", domain.MaximumListingTitleLength+1)},
		{name: "trimmed title", value: "  Neo 98  ", valid: true},
		{name: "blank title", value: " \t\n "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := domain.ValidateListingTitle(tt.value)
			if (err == nil) != tt.valid {
				t.Errorf("ValidateListingTitle(%q) error = %v, valid = %v", tt.value, err, tt.valid)
			}
		})
	}

	if err := domain.ValidateDescription(strings.Repeat("界", domain.MaximumDescriptionLength)); err != nil {
		t.Fatalf("description at boundary error = %v", err)
	}
	if err := domain.ValidateDescription(strings.Repeat("界", domain.MaximumDescriptionLength+1)); err == nil {
		t.Fatal("description over Unicode boundary was accepted")
	}
}

func TestListingValidationBoundaries(t *testing.T) {
	t.Parallel()

	for _, value := range []int64{domain.MinimumListingPriceIDR, domain.MaximumListingPriceIDR} {
		if err := domain.ValidatePriceIDR(value); err != nil {
			t.Errorf("price %d error = %v", value, err)
		}
	}
	for _, value := range []int64{domain.MinimumListingPriceIDR - 1, domain.MaximumListingPriceIDR + 1} {
		if err := domain.ValidatePriceIDR(value); err == nil {
			t.Errorf("invalid price %d was accepted", value)
		}
	}
	for _, value := range []int32{domain.MinimumListingQuantity, domain.MaximumListingQuantity} {
		if err := domain.ValidateQuantity(value); err != nil {
			t.Errorf("quantity %d error = %v", value, err)
		}
	}
	for _, value := range []int32{domain.MinimumListingQuantity - 1, domain.MaximumListingQuantity + 1} {
		if err := domain.ValidateQuantity(value); err == nil {
			t.Errorf("invalid quantity %d was accepted", value)
		}
	}

	for _, value := range []domain.ListingCondition{"", "refurbished", domain.ListingConditionNew, domain.ListingConditionUsed} {
		err := domain.ValidateListingCondition(value)
		if value == domain.ListingConditionNew || value == domain.ListingConditionUsed {
			if err != nil {
				t.Errorf("condition %q error = %v", value, err)
			}
			continue
		}
		var validation *domain.ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("condition %q error = %v, want ValidationError", value, err)
		}
	}
}
