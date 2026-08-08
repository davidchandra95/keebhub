package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumCategorySlugLength = 50
	MaximumCategoryNameLength = 100
	MaximumListingTitleLength = 120
	MaximumDescriptionLength  = 5000
	MinimumListingPriceIDR    = int64(1)
	MaximumListingPriceIDR    = int64(10_000_000_000)
	MinimumListingQuantity    = int32(1)
	MaximumListingQuantity    = int32(1_000_000)
)

const (
	ListingConditionNew  ListingCondition = "new"
	ListingConditionUsed ListingCondition = "used"

	ListingStatusActive   ListingStatus = "active"
	ListingStatusReserved ListingStatus = "reserved"
	ListingStatusSold     ListingStatus = "sold"
	ListingStatusArchived ListingStatus = "archived"

	ModerationStatusVisible ModerationStatus = "visible"
	ModerationStatusRemoved ModerationStatus = "removed"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)

type ListingCondition string
type ListingStatus string
type ModerationStatus string

type Category struct {
	ID        int64
	Slug      string
	Name      string
	SortOrder int32
	Active    bool
}

type PublicUser struct {
	ID          int64
	Handle      string
	DisplayName string
	AvatarURL   *string
	Location    *string
	Bio         *string
}

type Listing struct {
	ID               int64
	SellerID         int64
	CategoryID       int64
	Title            string
	Description      string
	PriceIDR         int64
	Quantity         int32
	Condition        ListingCondition
	Status           ListingStatus
	ModerationStatus ModerationStatus
	Negotiable       bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Category         Category
	Seller           PublicUser
}

// ValidationError identifies one or more invalid domain fields.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "validation failed"
}

// QueryError identifies invalid URL query values or opaque cursors.
type QueryError struct {
	Fields map[string]string
}

func (e *QueryError) Error() string {
	return "invalid query"
}

func NewValidationError(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func NewQueryError(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	return &QueryError{Fields: fields}
}

func NormalizeCategorySlug(value string) string {
	return strings.TrimSpace(value)
}

func ValidateListingTitle(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return NewValidationError(map[string]string{"title": "must not be blank"})
	}
	if utf8.RuneCountInString(trimmed) > MaximumListingTitleLength {
		return NewValidationError(map[string]string{"title": fmt.Sprintf("must be at most %d characters", MaximumListingTitleLength)})
	}
	return nil
}

func ValidateDescription(value string) error {
	if utf8.RuneCountInString(value) > MaximumDescriptionLength {
		return NewValidationError(map[string]string{"description": fmt.Sprintf("must be at most %d characters", MaximumDescriptionLength)})
	}
	return nil
}

func ValidatePriceIDR(value int64) error {
	if value < MinimumListingPriceIDR || value > MaximumListingPriceIDR {
		return NewValidationError(map[string]string{"price_idr": fmt.Sprintf("must be between %d and %d", MinimumListingPriceIDR, MaximumListingPriceIDR)})
	}
	return nil
}

func ValidateQuantity(value int32) error {
	if value < MinimumListingQuantity || value > MaximumListingQuantity {
		return NewValidationError(map[string]string{"quantity": fmt.Sprintf("must be between %d and %d", MinimumListingQuantity, MaximumListingQuantity)})
	}
	return nil
}

func ValidateListingCondition(value ListingCondition) error {
	if value != ListingConditionNew && value != ListingConditionUsed {
		return NewValidationError(map[string]string{"condition": "must be new or used"})
	}
	return nil
}

func ValidateListingStatus(value ListingStatus) error {
	switch value {
	case ListingStatusActive, ListingStatusReserved, ListingStatusSold, ListingStatusArchived:
		return nil
	default:
		return NewValidationError(map[string]string{"status": "must be active, reserved, sold, or archived"})
	}
}

func ValidateModerationStatus(value ModerationStatus) error {
	if value != ModerationStatusVisible && value != ModerationStatusRemoved {
		return NewValidationError(map[string]string{"moderation_status": "must be visible or removed"})
	}
	return nil
}

func CanTransitionListingStatus(from, to ListingStatus) bool {
	if from == to {
		return ValidateListingStatus(from) == nil
	}
	switch from {
	case ListingStatusActive:
		return to == ListingStatusReserved || to == ListingStatusSold || to == ListingStatusArchived
	case ListingStatusReserved:
		return to == ListingStatusActive || to == ListingStatusSold || to == ListingStatusArchived
	case ListingStatusSold:
		return to == ListingStatusActive || to == ListingStatusArchived
	case ListingStatusArchived:
		return to == ListingStatusActive
	default:
		return false
	}
}
