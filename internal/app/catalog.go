package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const (
	defaultListingPageSize = 20
	maximumListingPageSize = 100
	cursorVersion          = 1
	maximumCursorLength    = 2048
)

var ErrInvalidCatalogQuery = errors.New("invalid catalog query")

// CatalogQueryError identifies an invalid pagination or search option.
type CatalogQueryError struct {
	Field   string
	Message string
}

func (e *CatalogQueryError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

func (e *CatalogQueryError) Unwrap() error {
	return ErrInvalidCatalogQuery
}

// CategoryRepository contains the category persistence behavior consumed by
// CatalogService.
type CategoryRepository interface {
	ListActive(ctx context.Context) ([]domain.Category, error)
	FindActiveBySlug(ctx context.Context, slug string) (domain.Category, error)
}

// ListingRepository contains the listing persistence behavior consumed by
// CatalogService.
type ListingRepository interface {
	Create(ctx context.Context, params CreateListingParams) (domain.Listing, error)
	GetByID(ctx context.Context, listingID int64) (domain.Listing, error)
	UpdateOwned(ctx context.Context, params UpdateOwnedListingParams) (domain.Listing, error)
	ChangeOwnedStatus(ctx context.Context, params ChangeOwnedListingStatusParams) (domain.Listing, error)
	ListOwned(ctx context.Context, params ListOwnedListingsParams) ([]domain.Listing, error)
	Search(ctx context.Context, params SearchListingsParams) ([]domain.Listing, error)
}

type CatalogService struct {
	categories CategoryRepository
	listings   ListingRepository
	now        func() time.Time
}

func NewCatalogService(categories CategoryRepository, listings ListingRepository, now func() time.Time) *CatalogService {
	if now == nil {
		now = time.Now
	}
	return &CatalogService{
		categories: categories,
		listings:   listings,
		now:        now,
	}
}

type CreateListingInput struct {
	Title        string
	Description  string
	PriceIDR     int64
	Quantity     int
	CategorySlug string
	Condition    domain.ListingCondition
	Negotiable   bool
}

type CreateListingParams struct {
	SellerID         int64
	CategoryID       int64
	Title            string
	Description      string
	PriceIDR         int64
	Quantity         int
	Condition        domain.ListingCondition
	Status           domain.ListingStatus
	ModerationStatus domain.ModerationStatus
	Negotiable       bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UpdateListingInput struct {
	Title        *string
	Description  *string
	PriceIDR     *int64
	Quantity     *int
	CategorySlug *string
	Condition    *domain.ListingCondition
	Negotiable   *bool
}

type UpdateOwnedListingParams struct {
	ListingID   int64
	SellerID    int64
	Title       *string
	Description *string
	PriceIDR    *int64
	Quantity    *int
	CategoryID  *int64
	Condition   *domain.ListingCondition
	Negotiable  *bool
	UpdatedAt   time.Time
}

type ChangeOwnedListingStatusParams struct {
	ListingID int64
	SellerID  int64
	Status    domain.ListingStatus
	UpdatedAt time.Time
}

type OwnedListingsInput struct {
	Status string
	Cursor string
	Limit  int
}

type ListOwnedListingsParams struct {
	SellerID int64
	Status   *domain.ListingStatus
	Cursor   *OwnedListingCursor
	PageSize int
}

type OwnedListingCursor struct {
	UpdatedAt time.Time
	ID        int64
}

type ListingSort string

const (
	ListingSortNewest    ListingSort = "newest"
	ListingSortPriceAsc  ListingSort = "price_asc"
	ListingSortPriceDesc ListingSort = "price_desc"
)

type SearchListingsInput struct {
	Query     string
	Category  string
	Condition string
	MinPrice  *int64
	MaxPrice  *int64
	Sort      string
	Cursor    string
	Limit     int
}

type SearchListingsParams struct {
	Query     *string
	Category  *string
	Condition *domain.ListingCondition
	MinPrice  *int64
	MaxPrice  *int64
	Sort      ListingSort
	Cursor    *PublicListingCursor
	PageSize  int
}

type PublicListingCursor struct {
	CreatedAt time.Time
	PriceIDR  int64
	ID        int64
}

type ListingPage struct {
	Items      []domain.Listing
	NextCursor *string
}

type ListingStatusChange struct {
	Listing   domain.Listing
	OldStatus domain.ListingStatus
	Changed   bool
}

func (s *CatalogService) ListCategories(ctx context.Context) ([]domain.Category, error) {
	if s.categories == nil {
		return nil, errors.New("category repository is not configured")
	}
	categories, err := s.categories.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active categories: %w", err)
	}
	if categories == nil {
		return []domain.Category{}, nil
	}
	return categories, nil
}

func (s *CatalogService) CreateListing(ctx context.Context, seller domain.User, input CreateListingInput) (domain.Listing, error) {
	if err := canMutateCatalog(seller); err != nil {
		return domain.Listing{}, err
	}
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}

	title, err := domain.NormalizeListingTitle(input.Title)
	if err != nil {
		return domain.Listing{}, err
	}
	if err := domain.ValidateListingDescription(input.Description); err != nil {
		return domain.Listing{}, err
	}
	if err := domain.ValidateListingPriceIDR(input.PriceIDR); err != nil {
		return domain.Listing{}, err
	}
	if err := domain.ValidateListingQuantity(input.Quantity); err != nil {
		return domain.Listing{}, err
	}
	categorySlug, err := domain.NormalizeCategorySlug(input.CategorySlug)
	if err != nil {
		return domain.Listing{}, err
	}
	if err := domain.ValidateListingCondition(input.Condition); err != nil {
		return domain.Listing{}, err
	}

	category, err := s.activeCategory(ctx, categorySlug)
	if err != nil {
		return domain.Listing{}, err
	}
	now := s.now().UTC()
	listing, err := s.listings.Create(ctx, CreateListingParams{
		SellerID:         seller.ID,
		CategoryID:       category.ID,
		Title:            title,
		Description:      input.Description,
		PriceIDR:         input.PriceIDR,
		Quantity:         input.Quantity,
		Condition:        input.Condition,
		Status:           domain.ListingStatusActive,
		ModerationStatus: domain.ModerationStatusVisible,
		Negotiable:       input.Negotiable,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("create listing: %w", err)
	}
	return listing, nil
}

func (s *CatalogService) UpdateListing(ctx context.Context, seller domain.User, listingID int64, input UpdateListingInput) (domain.Listing, error) {
	if err := canMutateCatalog(seller); err != nil {
		return domain.Listing{}, err
	}
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}
	if listingID <= 0 {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if !hasListingUpdate(input) {
		return domain.Listing{}, &domain.ValidationError{Field: "body", Message: "must contain at least one field"}
	}

	if _, err := s.ownedVisibleListing(ctx, seller.ID, listingID); err != nil {
		return domain.Listing{}, err
	}
	params, err := s.prepareListingUpdate(ctx, seller.ID, listingID, input)
	if err != nil {
		return domain.Listing{}, err
	}
	params.UpdatedAt = s.now().UTC()
	listing, err := s.listings.UpdateOwned(ctx, params)
	if errors.Is(err, domain.ErrListingNotFound) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update owned listing: %w", err)
	}
	return listing, nil
}

func (s *CatalogService) ChangeListingStatus(ctx context.Context, seller domain.User, listingID int64, status domain.ListingStatus) (ListingStatusChange, error) {
	if err := canMutateCatalog(seller); err != nil {
		return ListingStatusChange{}, err
	}
	if s.listings == nil {
		return ListingStatusChange{}, errors.New("listing repository is not configured")
	}
	if listingID <= 0 {
		return ListingStatusChange{}, domain.ErrListingNotFound
	}

	listing, err := s.ownedVisibleListing(ctx, seller.ID, listingID)
	if err != nil {
		return ListingStatusChange{}, err
	}
	if err := domain.ValidateListingStatusTransition(listing.Status, status); err != nil {
		return ListingStatusChange{}, err
	}
	if listing.Status == status {
		return ListingStatusChange{Listing: listing, OldStatus: listing.Status}, nil
	}

	updated, err := s.listings.ChangeOwnedStatus(ctx, ChangeOwnedListingStatusParams{
		ListingID: listingID,
		SellerID:  seller.ID,
		Status:    status,
		UpdatedAt: s.now().UTC(),
	})
	if errors.Is(err, domain.ErrListingNotFound) {
		return ListingStatusChange{}, domain.ErrListingNotFound
	}
	if err != nil {
		return ListingStatusChange{}, fmt.Errorf("change owned listing status: %w", err)
	}
	return ListingStatusChange{Listing: updated, OldStatus: listing.Status, Changed: true}, nil
}

func (s *CatalogService) ListOwnedListings(ctx context.Context, seller domain.User, input OwnedListingsInput) (ListingPage, error) {
	if s.listings == nil {
		return ListingPage{}, errors.New("listing repository is not configured")
	}
	if seller.ID <= 0 {
		return ListingPage{}, domain.ErrForbidden
	}
	status, cursor, limit, err := normalizeOwnedListingsInput(input)
	if err != nil {
		return ListingPage{}, err
	}

	listings, err := s.listings.ListOwned(ctx, ListOwnedListingsParams{
		SellerID: seller.ID,
		Status:   status,
		Cursor:   cursor,
		PageSize: limit + 1,
	})
	if err != nil {
		return ListingPage{}, fmt.Errorf("list owned listings: %w", err)
	}
	return ownedListingPage(listings, limit, status)
}

func (s *CatalogService) GetListing(ctx context.Context, listingID int64, viewer *domain.User) (domain.Listing, error) {
	if s.listings == nil {
		return domain.Listing{}, errors.New("listing repository is not configured")
	}
	if listingID <= 0 {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	listing, err := s.listings.GetByID(ctx, listingID)
	if errors.Is(err, domain.ErrListingNotFound) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing: %w", err)
	}

	viewerID := int64(0)
	if viewer != nil {
		viewerID = viewer.ID
	}
	if !listing.IsVisibleTo(viewerID) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	return listing, nil
}

func (s *CatalogService) SearchListings(ctx context.Context, input SearchListingsInput) (ListingPage, error) {
	if s.listings == nil {
		return ListingPage{}, errors.New("listing repository is not configured")
	}
	params, query, category, condition, limit, err := normalizeSearchListingsInput(input)
	if err != nil {
		return ListingPage{}, err
	}
	listings, err := s.listings.Search(ctx, params)
	if err != nil {
		return ListingPage{}, fmt.Errorf("search listings: %w", err)
	}
	return publicListingPage(listings, limit, query, category, condition, params.MinPrice, params.MaxPrice, params.Sort)
}

func (s *CatalogService) activeCategory(ctx context.Context, slug string) (domain.Category, error) {
	category, err := s.categories.FindActiveBySlug(ctx, slug)
	if errors.Is(err, domain.ErrCategoryNotFound) {
		return domain.Category{}, &domain.ValidationError{Field: "category_slug", Message: "must refer to an active category"}
	}
	if err != nil {
		return domain.Category{}, fmt.Errorf("find active category: %w", err)
	}
	return category, nil
}

func (s *CatalogService) ownedVisibleListing(ctx context.Context, sellerID, listingID int64) (domain.Listing, error) {
	listing, err := s.listings.GetByID(ctx, listingID)
	if errors.Is(err, domain.ErrListingNotFound) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get owned listing: %w", err)
	}
	if listing.ModerationStatus == domain.ModerationStatusRemoved {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if listing.SellerID != sellerID {
		return domain.Listing{}, domain.ErrForbidden
	}
	return listing, nil
}

func (s *CatalogService) prepareListingUpdate(ctx context.Context, sellerID, listingID int64, input UpdateListingInput) (UpdateOwnedListingParams, error) {
	params := UpdateOwnedListingParams{ListingID: listingID, SellerID: sellerID}
	if input.Title != nil {
		value, err := domain.NormalizeListingTitle(*input.Title)
		if err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.Title = &value
	}
	if input.Description != nil {
		if err := domain.ValidateListingDescription(*input.Description); err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.Description = input.Description
	}
	if input.PriceIDR != nil {
		if err := domain.ValidateListingPriceIDR(*input.PriceIDR); err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.PriceIDR = input.PriceIDR
	}
	if input.Quantity != nil {
		if err := domain.ValidateListingQuantity(*input.Quantity); err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.Quantity = input.Quantity
	}
	if input.CategorySlug != nil {
		slug, err := domain.NormalizeCategorySlug(*input.CategorySlug)
		if err != nil {
			return UpdateOwnedListingParams{}, err
		}
		category, err := s.activeCategory(ctx, slug)
		if err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.CategoryID = &category.ID
	}
	if input.Condition != nil {
		if err := domain.ValidateListingCondition(*input.Condition); err != nil {
			return UpdateOwnedListingParams{}, err
		}
		params.Condition = input.Condition
	}
	if input.Negotiable != nil {
		params.Negotiable = input.Negotiable
	}
	return params, nil
}

func canMutateCatalog(user domain.User) error {
	if user.ID <= 0 {
		return domain.ErrForbidden
	}
	if user.Status == domain.UserStatusDisabled {
		return domain.ErrUserDisabled
	}
	return nil
}

func hasListingUpdate(input UpdateListingInput) bool {
	return input.Title != nil ||
		input.Description != nil ||
		input.PriceIDR != nil ||
		input.Quantity != nil ||
		input.CategorySlug != nil ||
		input.Condition != nil ||
		input.Negotiable != nil
}

func normalizeOwnedListingsInput(input OwnedListingsInput) (*domain.ListingStatus, *OwnedListingCursor, int, error) {
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return nil, nil, 0, err
	}
	var status *domain.ListingStatus
	if input.Status != "" {
		value := domain.ListingStatus(input.Status)
		if err := domain.ValidateListingStatus(value); err != nil {
			return nil, nil, 0, invalidCatalogQuery("status", "must be active, reserved, sold, or archived")
		}
		status = &value
	}
	cursor, err := decodeOwnedListingCursor(input.Cursor, status)
	if err != nil {
		return nil, nil, 0, err
	}
	return status, cursor, limit, nil
}

func normalizeSearchListingsInput(input SearchListingsInput) (SearchListingsParams, string, string, string, int, error) {
	limit, err := normalizePageLimit(input.Limit)
	if err != nil {
		return SearchListingsParams{}, "", "", "", 0, err
	}

	query := strings.TrimSpace(input.Query)
	if utf8.RuneCountInString(query) > 100 {
		return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("q", "must be at most 100 characters")
	}
	category := strings.TrimSpace(input.Category)
	if utf8.RuneCountInString(category) > domain.MaximumCategorySlugRunes {
		return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("category", "must be at most 50 characters")
	}

	var condition *domain.ListingCondition
	if input.Condition != "" {
		value := domain.ListingCondition(input.Condition)
		if err := domain.ValidateListingCondition(value); err != nil {
			return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("condition", "must be new or used")
		}
		condition = &value
	}
	if input.MinPrice != nil {
		if err := domain.ValidateListingPriceIDR(*input.MinPrice); err != nil {
			return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("min_price", "must be between 1 and 10000000000")
		}
	}
	if input.MaxPrice != nil {
		if err := domain.ValidateListingPriceIDR(*input.MaxPrice); err != nil {
			return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("max_price", "must be between 1 and 10000000000")
		}
	}
	if input.MinPrice != nil && input.MaxPrice != nil && *input.MinPrice > *input.MaxPrice {
		return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("min_price", "must not exceed max_price")
	}

	sort := ListingSort(input.Sort)
	if sort == "" {
		sort = ListingSortNewest
	}
	if !sort.Valid() {
		return SearchListingsParams{}, "", "", "", 0, invalidCatalogQuery("sort", "must be newest, price_asc, or price_desc")
	}

	cursor, err := decodePublicListingCursor(input.Cursor, query, category, condition, input.MinPrice, input.MaxPrice, sort)
	if err != nil {
		return SearchListingsParams{}, "", "", "", 0, err
	}

	params := SearchListingsParams{
		Condition: condition,
		MinPrice:  input.MinPrice,
		MaxPrice:  input.MaxPrice,
		Sort:      sort,
		Cursor:    cursor,
		PageSize:  limit + 1,
	}
	if query != "" {
		escaped := escapeILIKE(query)
		params.Query = &escaped
	}
	if category != "" {
		params.Category = &category
	}
	conditionText := ""
	if condition != nil {
		conditionText = string(*condition)
	}
	return params, query, category, conditionText, limit, nil
}

func (s ListingSort) Valid() bool {
	switch s {
	case ListingSortNewest, ListingSortPriceAsc, ListingSortPriceDesc:
		return true
	default:
		return false
	}
}

func normalizePageLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultListingPageSize, nil
	}
	if limit < 1 || limit > maximumListingPageSize {
		return 0, invalidCatalogQuery("limit", "must be between 1 and 100")
	}
	return limit, nil
}

func ownedListingPage(listings []domain.Listing, limit int, status *domain.ListingStatus) (ListingPage, error) {
	items, hasNext := listingPageItems(listings, limit)
	page := ListingPage{Items: items}
	if !hasNext {
		return page, nil
	}
	cursor, err := encodeOwnedListingCursor(status, items[len(items)-1])
	if err != nil {
		return ListingPage{}, err
	}
	page.NextCursor = &cursor
	return page, nil
}

func publicListingPage(
	listings []domain.Listing,
	limit int,
	query, category, condition string,
	minPrice, maxPrice *int64,
	sort ListingSort,
) (ListingPage, error) {
	items, hasNext := listingPageItems(listings, limit)
	page := ListingPage{Items: items}
	if !hasNext {
		return page, nil
	}
	cursor, err := encodePublicListingCursor(query, category, condition, minPrice, maxPrice, sort, items[len(items)-1])
	if err != nil {
		return ListingPage{}, err
	}
	page.NextCursor = &cursor
	return page, nil
}

func listingPageItems(listings []domain.Listing, limit int) ([]domain.Listing, bool) {
	if len(listings) <= limit {
		if listings == nil {
			return []domain.Listing{}, false
		}
		return listings, false
	}
	items := make([]domain.Listing, limit)
	copy(items, listings[:limit])
	return items, true
}

type ownedListingCursorPayload struct {
	Version   int    `json:"v"`
	Kind      string `json:"k"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	ID        int64  `json:"id"`
}

func encodeOwnedListingCursor(status *domain.ListingStatus, listing domain.Listing) (string, error) {
	statusValue := ""
	if status != nil {
		statusValue = string(*status)
	}
	return encodeCursor(ownedListingCursorPayload{
		Version:   cursorVersion,
		Kind:      "owned",
		Status:    statusValue,
		UpdatedAt: listing.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ID:        listing.ID,
	})
}

func decodeOwnedListingCursor(value string, status *domain.ListingStatus) (*OwnedListingCursor, error) {
	if value == "" {
		return nil, nil
	}
	var payload ownedListingCursorPayload
	if err := decodeCursor(value, &payload); err != nil {
		return nil, invalidCatalogQuery("cursor", "is malformed")
	}
	statusValue := ""
	if status != nil {
		statusValue = string(*status)
	}
	if payload.Version != cursorVersion || payload.Kind != "owned" || payload.Status != statusValue {
		return nil, invalidCatalogQuery("cursor", "does not match the selected status")
	}
	updatedAt, err := parseCursorTime(payload.UpdatedAt)
	if err != nil || payload.ID <= 0 {
		return nil, invalidCatalogQuery("cursor", "has an invalid position")
	}
	return &OwnedListingCursor{UpdatedAt: updatedAt, ID: payload.ID}, nil
}

type publicListingCursorPayload struct {
	Version   int         `json:"v"`
	Kind      string      `json:"k"`
	Query     string      `json:"q"`
	Category  string      `json:"category"`
	Condition string      `json:"condition"`
	MinPrice  *int64      `json:"min_price,omitempty"`
	MaxPrice  *int64      `json:"max_price,omitempty"`
	Sort      ListingSort `json:"sort"`
	CreatedAt string      `json:"created_at,omitempty"`
	PriceIDR  *int64      `json:"price_idr,omitempty"`
	ID        int64       `json:"id"`
}

func encodePublicListingCursor(
	query, category, condition string,
	minPrice, maxPrice *int64,
	sort ListingSort,
	listing domain.Listing,
) (string, error) {
	payload := publicListingCursorPayload{
		Version:   cursorVersion,
		Kind:      "public",
		Query:     query,
		Category:  category,
		Condition: condition,
		MinPrice:  minPrice,
		MaxPrice:  maxPrice,
		Sort:      sort,
		ID:        listing.ID,
	}
	if sort == ListingSortNewest {
		payload.CreatedAt = listing.CreatedAt.UTC().Format(time.RFC3339Nano)
	} else {
		price := listing.PriceIDR
		payload.PriceIDR = &price
	}
	return encodeCursor(payload)
}

func decodePublicListingCursor(
	value, query, category string,
	condition *domain.ListingCondition,
	minPrice, maxPrice *int64,
	sort ListingSort,
) (*PublicListingCursor, error) {
	if value == "" {
		return nil, nil
	}
	var payload publicListingCursorPayload
	if err := decodeCursor(value, &payload); err != nil {
		return nil, invalidCatalogQuery("cursor", "is malformed")
	}
	conditionValue := ""
	if condition != nil {
		conditionValue = string(*condition)
	}
	if payload.Version != cursorVersion || payload.Kind != "public" || payload.Query != query ||
		payload.Category != category || payload.Condition != conditionValue ||
		!sameOptionalInt64(payload.MinPrice, minPrice) || !sameOptionalInt64(payload.MaxPrice, maxPrice) ||
		payload.Sort != sort {
		return nil, invalidCatalogQuery("cursor", "does not match the selected filters or sort")
	}
	if payload.ID <= 0 {
		return nil, invalidCatalogQuery("cursor", "has an invalid position")
	}

	switch sort {
	case ListingSortNewest:
		if payload.PriceIDR != nil {
			return nil, invalidCatalogQuery("cursor", "has an invalid position")
		}
		createdAt, err := parseCursorTime(payload.CreatedAt)
		if err != nil {
			return nil, invalidCatalogQuery("cursor", "has an invalid position")
		}
		return &PublicListingCursor{CreatedAt: createdAt, ID: payload.ID}, nil
	case ListingSortPriceAsc, ListingSortPriceDesc:
		if payload.CreatedAt != "" || payload.PriceIDR == nil {
			return nil, invalidCatalogQuery("cursor", "has an invalid position")
		}
		if err := domain.ValidateListingPriceIDR(*payload.PriceIDR); err != nil {
			return nil, invalidCatalogQuery("cursor", "has an invalid position")
		}
		return &PublicListingCursor{PriceIDR: *payload.PriceIDR, ID: payload.ID}, nil
	default:
		return nil, invalidCatalogQuery("cursor", "has an invalid sort")
	}
}

func encodeCursor(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(value string, destination any) error {
	if len(value) > maximumCursorLength {
		return errors.New("cursor exceeds maximum length")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, destination)
}

func parseCursorTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("cursor timestamp is empty")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() {
		return time.Time{}, errors.New("cursor timestamp is invalid")
	}
	return parsed.UTC(), nil
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func escapeILIKE(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"%", "\\%",
		"_", "\\_",
	)
	return replacer.Replace(value)
}

func invalidCatalogQuery(field, message string) error {
	return &CatalogQueryError{Field: field, Message: message}
}
