package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinimumListingPriceIDR         int64 = 1
	MaximumListingPriceIDR         int64 = 10_000_000_000
	MinimumListingQuantity               = 1
	MaximumListingQuantity               = 1_000_000
	MaximumListingTitleRunes             = 120
	MaximumListingDescriptionRunes       = 5_000
	MaximumCategorySlugRunes             = 50
)

var (
	ErrListingNotFound  = errors.New("listing not found")
	ErrCategoryNotFound = errors.New("category not found")
	ErrForbidden        = errors.New("operation forbidden")
	ErrConflict         = errors.New("conflict")
)

// ValidationError identifies an invalid domain field without tying validation
// to a transport format.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

// ConflictError identifies a requested state change that conflicts with the
// current domain state.
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

func (e *ConflictError) Unwrap() error {
	return ErrConflict
}

type ListingCondition string

const (
	ListingConditionNew  ListingCondition = "new"
	ListingConditionUsed ListingCondition = "used"
)

type ListingStatus string

const (
	ListingStatusActive   ListingStatus = "active"
	ListingStatusReserved ListingStatus = "reserved"
	ListingStatusSold     ListingStatus = "sold"
	ListingStatusArchived ListingStatus = "archived"
)

type ModerationStatus string

const (
	ModerationStatusVisible ModerationStatus = "visible"
	ModerationStatusRemoved ModerationStatus = "removed"
)

type Category struct {
	ID   int64
	Slug string
	Name string
}

type PublicUser struct {
	ID          int64
	Handle      string
	DisplayName string
	AvatarURL   *string
	Location    *string
}

type Listing struct {
	ID               int64
	SellerID         int64
	CategoryID       int64
	Title            string
	Description      string
	PriceIDR         int64
	Quantity         int
	Category         Category
	Condition        ListingCondition
	Status           ListingStatus
	ModerationStatus ModerationStatus
	Negotiable       bool
	Seller           PublicUser
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NormalizeListingTitle(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", validationError("title", "must not be blank")
	}
	if utf8.RuneCountInString(normalized) > MaximumListingTitleRunes {
		return "", validationError("title", "must be at most 120 characters")
	}
	return normalized, nil
}

func NormalizeCategorySlug(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", validationError("category_slug", "must not be blank")
	}
	if utf8.RuneCountInString(normalized) > MaximumCategorySlugRunes {
		return "", validationError("category_slug", "must be at most 50 characters")
	}
	return normalized, nil
}

func ValidateListingDescription(value string) error {
	if utf8.RuneCountInString(value) > MaximumListingDescriptionRunes {
		return validationError("description", "must be at most 5000 characters")
	}
	return nil
}

func ValidateListingPriceIDR(value int64) error {
	if value < MinimumListingPriceIDR || value > MaximumListingPriceIDR {
		return validationError("price_idr", "must be between 1 and 10000000000")
	}
	return nil
}

func ValidateListingQuantity(value int) error {
	if value < MinimumListingQuantity || value > MaximumListingQuantity {
		return validationError("quantity", "must be between 1 and 1000000")
	}
	return nil
}

func ValidateListingCondition(value ListingCondition) error {
	switch value {
	case ListingConditionNew, ListingConditionUsed:
		return nil
	default:
		return validationError("condition", "must be new or used")
	}
}

func ValidateListingStatus(value ListingStatus) error {
	switch value {
	case ListingStatusActive, ListingStatusReserved, ListingStatusSold, ListingStatusArchived:
		return nil
	default:
		return validationError("status", "must be active, reserved, sold, or archived")
	}
}

func ValidateModerationStatus(value ModerationStatus) error {
	switch value {
	case ModerationStatusVisible, ModerationStatusRemoved:
		return nil
	default:
		return validationError("moderation_status", "must be visible or removed")
	}
}

func ValidateListingStatusTransition(current, next ListingStatus) error {
	if err := ValidateListingStatus(current); err != nil {
		return err
	}
	if err := ValidateListingStatus(next); err != nil {
		return err
	}
	if current == next {
		return nil
	}

	allowed := map[ListingStatus]map[ListingStatus]bool{
		ListingStatusActive: {
			ListingStatusReserved: true,
			ListingStatusSold:     true,
			ListingStatusArchived: true,
		},
		ListingStatusReserved: {
			ListingStatusActive:   true,
			ListingStatusSold:     true,
			ListingStatusArchived: true,
		},
		ListingStatusSold: {
			ListingStatusActive:   true,
			ListingStatusArchived: true,
		},
		ListingStatusArchived: {
			ListingStatusActive: true,
		},
	}
	if allowed[current][next] {
		return nil
	}
	return &ConflictError{Message: fmt.Sprintf("cannot change listing status from %s to %s", current, next)}
}

func (l Listing) IsVisibleTo(viewerID int64) bool {
	if l.ModerationStatus == ModerationStatusRemoved {
		return false
	}
	return l.Status != ListingStatusArchived || viewerID == l.SellerID
}

func validationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
