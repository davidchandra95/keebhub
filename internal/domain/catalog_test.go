package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestListingValidationUsesUnicodeCharacterBoundaries(t *testing.T) {
	t.Parallel()

	titleAtLimit := strings.Repeat("界", MaximumListingTitleRunes)
	titleOverLimit := titleAtLimit + "界"
	descriptionAtLimit := strings.Repeat("界", MaximumListingDescriptionRunes)
	descriptionOverLimit := descriptionAtLimit + "界"

	title, err := NormalizeListingTitle(" \n" + titleAtLimit + "\t")
	if err != nil {
		t.Fatalf("normalize title at limit: %v", err)
	}
	if title != titleAtLimit {
		t.Errorf("normalized title = %q, want trimmed value", title)
	}
	if _, err := NormalizeListingTitle(titleOverLimit); err == nil {
		t.Error("expected title over Unicode character limit to fail")
	}
	if _, err := NormalizeListingTitle(" \u2003 "); err == nil {
		t.Error("expected Unicode whitespace-only title to fail")
	}
	if err := ValidateListingDescription(descriptionAtLimit); err != nil {
		t.Fatalf("validate description at limit: %v", err)
	}
	if err := ValidateListingDescription(descriptionOverLimit); err == nil {
		t.Error("expected description over Unicode character limit to fail")
	}
}

func TestListingValueBoundaries(t *testing.T) {
	t.Parallel()

	for _, price := range []int64{MinimumListingPriceIDR, MaximumListingPriceIDR} {
		if err := ValidateListingPriceIDR(price); err != nil {
			t.Errorf("price %d: %v", price, err)
		}
	}
	for _, price := range []int64{0, MaximumListingPriceIDR + 1} {
		if err := ValidateListingPriceIDR(price); err == nil {
			t.Errorf("price %d unexpectedly succeeded", price)
		}
	}
	for _, quantity := range []int{MinimumListingQuantity, MaximumListingQuantity} {
		if err := ValidateListingQuantity(quantity); err != nil {
			t.Errorf("quantity %d: %v", quantity, err)
		}
	}
	for _, quantity := range []int{0, MaximumListingQuantity + 1} {
		if err := ValidateListingQuantity(quantity); err == nil {
			t.Errorf("quantity %d unexpectedly succeeded", quantity)
		}
	}
}

func TestListingEnumsRejectUnknownValues(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "condition", err: ValidateListingCondition("refurbished")},
		{name: "status", err: ValidateListingStatus("paused")},
		{name: "moderation", err: ValidateModerationStatus("pending")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var validationError *ValidationError
			if !errors.As(test.err, &validationError) {
				t.Errorf("error = %v, want ValidationError", test.err)
			}
		})
	}
}

func TestListingStatusTransitions(t *testing.T) {
	t.Parallel()

	statuses := []ListingStatus{
		ListingStatusActive,
		ListingStatusReserved,
		ListingStatusSold,
		ListingStatusArchived,
	}
	allowed := map[ListingStatus]map[ListingStatus]bool{
		ListingStatusActive: {
			ListingStatusActive:   true,
			ListingStatusReserved: true,
			ListingStatusSold:     true,
			ListingStatusArchived: true,
		},
		ListingStatusReserved: {
			ListingStatusActive:   true,
			ListingStatusReserved: true,
			ListingStatusSold:     true,
			ListingStatusArchived: true,
		},
		ListingStatusSold: {
			ListingStatusActive:   true,
			ListingStatusSold:     true,
			ListingStatusArchived: true,
		},
		ListingStatusArchived: {
			ListingStatusActive:   true,
			ListingStatusArchived: true,
		},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			from, to := from, to
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()

				err := ValidateListingStatusTransition(from, to)
				if allowed[from][to] {
					if err != nil {
						t.Fatalf("transition %s -> %s: %v", from, to, err)
					}
					return
				}
				if !errors.Is(err, ErrConflict) {
					t.Errorf("transition %s -> %s error = %v, want conflict", from, to, err)
				}
			})
		}
	}
}

func TestListingVisibility(t *testing.T) {
	t.Parallel()

	listing := Listing{SellerID: 42, Status: ListingStatusArchived, ModerationStatus: ModerationStatusVisible}
	if listing.IsVisibleTo(0) {
		t.Error("archived listing is visible anonymously")
	}
	if !listing.IsVisibleTo(42) {
		t.Error("archived listing is not visible to owner")
	}
	listing.ModerationStatus = ModerationStatusRemoved
	if listing.IsVisibleTo(42) {
		t.Error("removed listing is visible to owner")
	}
}
