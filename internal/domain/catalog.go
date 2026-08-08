package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumCategorySlugLength             = 50
	MaximumCategoryNameLength             = 100
	MaximumListingTitleLength             = 120
	MaximumListingDescriptionLength       = 5000
	MinimumListingPriceIDR          int64 = 1
	MaximumListingPriceIDR          int64 = 10000000000
	MinimumListingQuantity          int32 = 1
	MaximumListingQuantity          int32 = 1000000
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)

// ValidationError contains safe, field-specific validation messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "validation failed"
	}
	fields := make([]string, 0, len(e.Fields))
	for field := range e.Fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return "validation failed: " + strings.Join(fields, ", ")
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

var ErrValidation = errors.New("validation failed")

// InvalidStatusTransitionError identifies a valid status change that is not allowed.
type InvalidStatusTransitionError struct {
	From ListingStatus
	To   ListingStatus
}

func (e *InvalidStatusTransitionError) Error() string {
	return fmt.Sprintf("listing status transition from %q to %q is not allowed", e.From, e.To)
}

func (e *InvalidStatusTransitionError) Unwrap() error {
	return ErrConflict
}

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
}

type ListingCondition string

const (
	ConditionNew  ListingCondition = "new"
	ConditionUsed ListingCondition = "used"

	ListingConditionNew  = ConditionNew
	ListingConditionUsed = ConditionUsed
)

type ListingStatus string

const (
	StatusActive   ListingStatus = "active"
	StatusReserved ListingStatus = "reserved"
	StatusSold     ListingStatus = "sold"
	StatusArchived ListingStatus = "archived"

	ListingStatusActive   = StatusActive
	ListingStatusReserved = StatusReserved
	ListingStatusSold     = StatusSold
	ListingStatusArchived = StatusArchived
)

type ModerationStatus string

const (
	ModerationVisible ModerationStatus = "visible"
	ModerationRemoved ModerationStatus = "removed"

	ListingModerationVisible = ModerationVisible
	ListingModerationRemoved = ModerationRemoved
)

type Listing struct {
	ID               int64
	SellerID         int64
	Category         Category
	Title            string
	Description      string
	PriceIDR         int64
	Quantity         int32
	Condition        ListingCondition
	Status           ListingStatus
	ModerationStatus ModerationStatus
	Negotiable       bool
	Seller           PublicUser
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NormalizeCategorySlug(value string) string {
	return strings.TrimSpace(value)
}

func NormalizeListingTitle(value string) string {
	return strings.TrimSpace(value)
}

func ValidateCategorySlug(value string) error {
	value = NormalizeCategorySlug(value)
	if value == "" {
		return &ValidationError{Fields: map[string]string{"category_slug": "Category is required."}}
	}
	if !utf8.ValidString(value) {
		return &ValidationError{Fields: map[string]string{"category_slug": "Category is invalid."}}
	}
	if utf8.RuneCountInString(value) > MaximumCategorySlugLength {
		return &ValidationError{Fields: map[string]string{"category_slug": "Category is too long."}}
	}
	return nil
}

func ValidateListingTitle(value string) error {
	value = NormalizeListingTitle(value)
	if value == "" {
		return &ValidationError{Fields: map[string]string{"title": "Title is required."}}
	}
	if !utf8.ValidString(value) {
		return &ValidationError{Fields: map[string]string{"title": "Title is invalid."}}
	}
	if utf8.RuneCountInString(value) > MaximumListingTitleLength {
		return &ValidationError{Fields: map[string]string{"title": "Title is too long."}}
	}
	return nil
}

func ValidateListingDescription(value string) error {
	if !utf8.ValidString(value) {
		return &ValidationError{Fields: map[string]string{"description": "Description is invalid."}}
	}
	if utf8.RuneCountInString(value) > MaximumListingDescriptionLength {
		return &ValidationError{Fields: map[string]string{"description": "Description is too long."}}
	}
	return nil
}

func ValidateListingPrice(value int64) error {
	if value < MinimumListingPriceIDR || value > MaximumListingPriceIDR {
		return &ValidationError{Fields: map[string]string{"price_idr": "Price must be between 1 and 10,000,000,000 IDR."}}
	}
	return nil
}

func ValidateListingQuantity(value int32) error {
	if value < MinimumListingQuantity || value > MaximumListingQuantity {
		return &ValidationError{Fields: map[string]string{"quantity": "Quantity must be between 1 and 1,000,000."}}
	}
	return nil
}

func ValidateListingCondition(value ListingCondition) error {
	switch value {
	case ConditionNew, ConditionUsed:
		return nil
	default:
		return &ValidationError{Fields: map[string]string{"condition": "Condition must be new or used."}}
	}
}

func ValidateListingStatus(value ListingStatus) error {
	switch value {
	case StatusActive, StatusReserved, StatusSold, StatusArchived:
		return nil
	default:
		return &ValidationError{Fields: map[string]string{"status": "Status is invalid."}}
	}
}

func ValidateModerationStatus(value ModerationStatus) error {
	switch value {
	case ModerationVisible, ModerationRemoved:
		return nil
	default:
		return &ValidationError{Fields: map[string]string{"moderation_status": "Moderation status is invalid."}}
	}
}

func ValidateListingFields(title, description string, priceIDR int64, quantity int32, condition ListingCondition) error {
	fields := make(map[string]string)
	collectValidation(fields, "title", ValidateListingTitle(title))
	collectValidation(fields, "description", ValidateListingDescription(description))
	collectValidation(fields, "price_idr", ValidateListingPrice(priceIDR))
	collectValidation(fields, "quantity", ValidateListingQuantity(quantity))
	collectValidation(fields, "condition", ValidateListingCondition(condition))
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func CanTransitionListingStatus(from, to ListingStatus) error {
	if err := ValidateListingStatus(from); err != nil {
		return err
	}
	if err := ValidateListingStatus(to); err != nil {
		return err
	}
	if from == to {
		return nil
	}

	allowed := map[ListingStatus]map[ListingStatus]bool{
		StatusActive: {
			StatusReserved: true,
			StatusSold:     true,
			StatusArchived: true,
		},
		StatusReserved: {
			StatusActive:   true,
			StatusSold:     true,
			StatusArchived: true,
		},
		StatusSold: {
			StatusActive:   true,
			StatusArchived: true,
		},
		StatusArchived: {
			StatusActive: true,
		},
	}
	if allowed[from][to] {
		return nil
	}
	return &InvalidStatusTransitionError{From: from, To: to}
}

func collectValidation(fields map[string]string, field string, err error) {
	if err == nil {
		return
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		for key, message := range validation.Fields {
			fields[key] = message
		}
		return
	}
	fields[field] = "Value is invalid."
}
