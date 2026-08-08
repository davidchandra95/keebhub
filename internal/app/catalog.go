package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const (
	DefaultListingPageLimit  = 20
	MaximumListingPageLimit  = 100
	cursorVersion            = 1
	ownedListingsCursorKind  = "owned_listings"
	publicListingsCursorKind = "public_listings"
)

var ErrBadRequest = errors.New("bad request")

// BadRequestError represents a safe request or cursor error that belongs at the HTTP 400 boundary.
type BadRequestError struct {
	Code    string
	Message string
	Fields  map[string]string
	Err     error
}

func (e *BadRequestError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *BadRequestError) Unwrap() error {
	if e.Err != nil {
		return e.Err
	}
	return ErrBadRequest
}

type CategoryRepository interface {
	ListActiveCategories(context.Context) ([]domain.Category, error)
	GetActiveCategoryBySlug(context.Context, string) (domain.Category, error)
}

type ListingRepository interface {
	CreateListing(context.Context, ListingCreateParams) (domain.Listing, error)
	GetListing(context.Context, int64) (domain.Listing, error)
	UpdateListing(context.Context, ListingUpdateParams) (domain.Listing, error)
	ChangeListingStatus(context.Context, ListingStatusChangeParams) (domain.Listing, error)
	ListOwnedListings(context.Context, OwnedListingsParams) ([]domain.Listing, error)
	SearchListings(context.Context, SearchListingsParams) ([]domain.Listing, error)
}

type ListingCreateParams struct {
	SellerID    int64
	CategoryID  int64
	Title       string
	Description string
	PriceIDR    int64
	Quantity    int32
	Condition   domain.ListingCondition
	Negotiable  bool
	CreatedAt   time.Time
}

type ListingUpdateParams struct {
	ID          int64
	SellerID    int64
	Title       *string
	Description *string
	PriceIDR    *int64
	Quantity    *int32
	CategoryID  *int64
	Condition   *domain.ListingCondition
	Negotiable  *bool
	UpdatedAt   time.Time
}

type ListingStatusChangeParams struct {
	ID        int64
	SellerID  int64
	Status    domain.ListingStatus
	UpdatedAt time.Time
}

type OwnedListingsParams struct {
	SellerID        int64
	Status          *domain.ListingStatus
	CursorUpdatedAt *time.Time
	CursorID        *int64
	Limit           int
}

type ListingSort string

const (
	SortNewest    ListingSort = "newest"
	SortPriceAsc  ListingSort = "price_asc"
	SortPriceDesc ListingSort = "price_desc"
)

type SearchListingsParams struct {
	Query           *string
	CategoryID      *int64
	Condition       *domain.ListingCondition
	MinPrice        *int64
	MaxPrice        *int64
	Sort            ListingSort
	CursorCreatedAt *time.Time
	CursorPriceIDR  *int64
	CursorID        *int64
	Limit           int
}

type CreateListingInput struct {
	Title        string
	Description  string
	PriceIDR     int64
	Quantity     int32
	QuantitySet  bool
	CategorySlug string
	Condition    domain.ListingCondition
	Negotiable   bool
}

type UpdateListingInput struct {
	Title        *string
	Description  *string
	PriceIDR     *int64
	Quantity     *int32
	CategorySlug *string
	Condition    *domain.ListingCondition
	Negotiable   *bool
}

type SearchListingsInput struct {
	Query     string
	Category  string
	Condition *domain.ListingCondition
	MinPrice  *int64
	MaxPrice  *int64
	Sort      ListingSort
	Cursor    string
	Limit     int
}

type ListOwnedListingsInput struct {
	Status domain.ListingStatus
	Cursor string
	Limit  int
}

type ListingPage struct {
	Items      []domain.Listing
	NextCursor *string
}

type CatalogService struct {
	categories CategoryRepository
	listings   ListingRepository
	now        func() time.Time
}

func NewCatalogService(categories CategoryRepository, listings ListingRepository) *CatalogService {
	return NewCatalogServiceWithClock(categories, listings, time.Now)
}

func NewCatalogServiceWithClock(categories CategoryRepository, listings ListingRepository, now func() time.Time) *CatalogService {
	if now == nil {
		now = time.Now
	}
	return &CatalogService{categories: categories, listings: listings, now: now}
}

func (s *CatalogService) ListCategories(ctx context.Context) ([]domain.Category, error) {
	if s.categories == nil {
		return nil, errors.New("category repository is not configured")
	}
	items, err := s.categories.ListActiveCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active categories: %w", err)
	}
	if items == nil {
		items = []domain.Category{}
	}
	return items, nil
}

func (s *CatalogService) CreateListing(ctx context.Context, sellerID int64, input CreateListingInput) (domain.Listing, error) {
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}
	if sellerID <= 0 {
		return domain.Listing{}, &domain.ValidationError{Fields: map[string]string{"seller_id": "Seller is invalid."}}
	}

	title := domain.NormalizeListingTitle(input.Title)
	categorySlug := domain.NormalizeCategorySlug(input.CategorySlug)
	quantity := input.Quantity
	if quantity == 0 && !input.QuantitySet {
		quantity = domain.MinimumListingQuantity
	}
	fields := make(map[string]string)
	mergeValidation(fields, domain.ValidateListingTitle(title))
	mergeValidation(fields, domain.ValidateListingDescription(input.Description))
	mergeValidation(fields, domain.ValidateListingPrice(input.PriceIDR))
	mergeValidation(fields, domain.ValidateListingQuantity(quantity))
	mergeValidation(fields, domain.ValidateListingCondition(input.Condition))
	mergeValidation(fields, domain.ValidateCategorySlug(categorySlug))
	if len(fields) > 0 {
		return domain.Listing{}, &domain.ValidationError{Fields: fields}
	}

	category, err := s.categories.GetActiveCategoryBySlug(ctx, categorySlug)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, &domain.ValidationError{Fields: map[string]string{"category_slug": "Category is not active."}}
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("resolve listing category: %w", err)
	}

	createdAt := s.now().UTC()
	listing, err := s.listings.CreateListing(ctx, ListingCreateParams{
		SellerID:    sellerID,
		CategoryID:  category.ID,
		Title:       title,
		Description: input.Description,
		PriceIDR:    input.PriceIDR,
		Quantity:    quantity,
		Condition:   input.Condition,
		Negotiable:  input.Negotiable,
		CreatedAt:   createdAt,
	})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("create listing: %w", err)
	}
	return listing, nil
}

func (s *CatalogService) UpdateListing(ctx context.Context, sellerID, listingID int64, input UpdateListingInput) (domain.Listing, error) {
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}
	if err := validatePositiveIDs(sellerID, listingID); err != nil {
		return domain.Listing{}, err
	}
	if input.Title == nil && input.Description == nil && input.PriceIDR == nil && input.Quantity == nil && input.CategorySlug == nil && input.Condition == nil && input.Negotiable == nil {
		return domain.Listing{}, &domain.ValidationError{Fields: map[string]string{"listing": "At least one field is required."}}
	}

	if _, err := s.ensureOwnerCanMutate(ctx, sellerID, listingID); err != nil {
		return domain.Listing{}, err
	}
	params := ListingUpdateParams{ID: listingID, SellerID: sellerID, UpdatedAt: s.now().UTC()}
	fields := make(map[string]string)
	if input.Title != nil {
		title := domain.NormalizeListingTitle(*input.Title)
		mergeValidation(fields, domain.ValidateListingTitle(title))
		params.Title = &title
	}
	if input.Description != nil {
		mergeValidation(fields, domain.ValidateListingDescription(*input.Description))
		params.Description = input.Description
	}
	if input.PriceIDR != nil {
		mergeValidation(fields, domain.ValidateListingPrice(*input.PriceIDR))
		params.PriceIDR = input.PriceIDR
	}
	if input.Quantity != nil {
		mergeValidation(fields, domain.ValidateListingQuantity(*input.Quantity))
		params.Quantity = input.Quantity
	}
	if input.Condition != nil {
		mergeValidation(fields, domain.ValidateListingCondition(*input.Condition))
		params.Condition = input.Condition
	}
	if input.Negotiable != nil {
		params.Negotiable = input.Negotiable
	}
	if input.CategorySlug != nil {
		categorySlug := domain.NormalizeCategorySlug(*input.CategorySlug)
		categoryErr := domain.ValidateCategorySlug(categorySlug)
		mergeValidation(fields, categoryErr)
		if categoryErr == nil {
			category, err := s.categories.GetActiveCategoryBySlug(ctx, categorySlug)
			switch {
			case errors.Is(err, domain.ErrNotFound):
				fields["category_slug"] = "Category is not active."
			case err != nil:
				return domain.Listing{}, fmt.Errorf("resolve listing category: %w", err)
			default:
				params.CategoryID = &category.ID
			}
		}
	}
	if len(fields) > 0 {
		return domain.Listing{}, &domain.ValidationError{Fields: fields}
	}

	listing, err := s.listings.UpdateListing(ctx, params)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update listing: %w", err)
	}
	return listing, nil
}

func (s *CatalogService) ChangeListingStatus(ctx context.Context, sellerID, listingID int64, status domain.ListingStatus) (domain.Listing, error) {
	if s.listings == nil {
		return domain.Listing{}, errors.New("listing repository is not configured")
	}
	if err := validatePositiveIDs(sellerID, listingID); err != nil {
		return domain.Listing{}, err
	}
	if err := domain.ValidateListingStatus(status); err != nil {
		return domain.Listing{}, err
	}
	listing, err := s.ensureOwnerCanMutate(ctx, sellerID, listingID)
	if err != nil {
		return domain.Listing{}, err
	}
	if err := domain.CanTransitionListingStatus(listing.Status, status); err != nil {
		return domain.Listing{}, err
	}
	if listing.Status == status {
		return listing, nil
	}

	updated, err := s.listings.ChangeListingStatus(ctx, ListingStatusChangeParams{
		ID: listingID, SellerID: sellerID, Status: status, UpdatedAt: s.now().UTC(),
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("change listing status: %w", err)
	}
	return updated, nil
}

func (s *CatalogService) ListOwnedListings(ctx context.Context, sellerID int64, input ListOwnedListingsInput) (ListingPage, error) {
	if s.listings == nil {
		return ListingPage{}, errors.New("listing repository is not configured")
	}
	if sellerID <= 0 {
		return ListingPage{}, &domain.ValidationError{Fields: map[string]string{"seller_id": "Seller is invalid."}}
	}
	limit, err := normalizeLimit(input.Limit)
	if err != nil {
		return ListingPage{}, err
	}
	var status *domain.ListingStatus
	if input.Status != "" {
		if err := domain.ValidateListingStatus(input.Status); err != nil {
			return ListingPage{}, newBadRequest("status", "Status is invalid.")
		}
		statusValue := input.Status
		status = &statusValue
	}

	var cursor ownedListingsCursor
	if input.Cursor != "" {
		if len(input.Cursor) > 2048 {
			return ListingPage{}, invalidCursorError("cursor is too long")
		}
		cursor, err = decodeOwnedListingsCursor(input.Cursor)
		if err != nil {
			return ListingPage{}, err
		}
		boundStatus := ""
		if status != nil {
			boundStatus = string(*status)
		}
		if cursor.Status != boundStatus {
			return ListingPage{}, invalidCursorError("cursor does not match the requested status filter")
		}
	}

	params := OwnedListingsParams{SellerID: sellerID, Status: status, Limit: limit + 1}
	if input.Cursor != "" {
		params.CursorUpdatedAt = &cursor.UpdatedAt
		params.CursorID = &cursor.ID
	}
	items, err := s.listings.ListOwnedListings(ctx, params)
	if err != nil {
		return ListingPage{}, fmt.Errorf("list owned listings: %w", err)
	}
	return pageFromOwnedListings(items, limit, status), nil
}

func (s *CatalogService) GetListing(ctx context.Context, listingID int64, viewerID *int64) (domain.Listing, error) {
	if s.listings == nil {
		return domain.Listing{}, errors.New("listing repository is not configured")
	}
	if listingID <= 0 {
		return domain.Listing{}, domain.ErrNotFound
	}
	listing, err := s.listings.GetListing(ctx, listingID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing: %w", err)
	}
	if listing.ModerationStatus == domain.ModerationRemoved {
		return domain.Listing{}, domain.ErrNotFound
	}
	if listing.Status == domain.StatusArchived && (viewerID == nil || *viewerID != listing.SellerID) {
		return domain.Listing{}, domain.ErrNotFound
	}
	return listing, nil
}

func (s *CatalogService) SearchListings(ctx context.Context, input SearchListingsInput) (ListingPage, error) {
	if s.categories == nil || s.listings == nil {
		return ListingPage{}, errors.New("catalog repositories are not configured")
	}
	limit, err := normalizeLimit(input.Limit)
	if err != nil {
		return ListingPage{}, err
	}
	query := strings.TrimSpace(input.Query)
	categorySlug := strings.TrimSpace(input.Category)
	if len([]rune(query)) > 100 {
		return ListingPage{}, newBadRequest("q", "Search text is too long.")
	}
	if categorySlug != "" {
		if err := domain.ValidateCategorySlug(categorySlug); err != nil {
			return ListingPage{}, newBadRequest("category", "Category filter is invalid.")
		}
	}
	if input.Condition != nil {
		if err := domain.ValidateListingCondition(*input.Condition); err != nil {
			return ListingPage{}, newBadRequest("condition", "Condition filter is invalid.")
		}
	}
	if input.MinPrice != nil {
		if err := domain.ValidateListingPrice(*input.MinPrice); err != nil {
			return ListingPage{}, newBadRequest("min_price", "Minimum price is invalid.")
		}
	}
	if input.MaxPrice != nil {
		if err := domain.ValidateListingPrice(*input.MaxPrice); err != nil {
			return ListingPage{}, newBadRequest("max_price", "Maximum price is invalid.")
		}
	}
	if input.MinPrice != nil && input.MaxPrice != nil && *input.MinPrice > *input.MaxPrice {
		return ListingPage{}, newBadRequest("max_price", "Maximum price must be greater than or equal to minimum price.")
	}
	sortValue := input.Sort
	if sortValue == "" {
		sortValue = SortNewest
	}
	if !validListingSort(sortValue) {
		return ListingPage{}, newBadRequest("sort", "Sort is invalid.")
	}

	var cursor searchListingsCursor
	if input.Cursor != "" {
		if len(input.Cursor) > 2048 {
			return ListingPage{}, invalidCursorError("cursor is too long")
		}
		cursor, err = decodeSearchListingsCursor(input.Cursor)
		if err != nil {
			return ListingPage{}, err
		}
		if !cursorMatchesSearch(cursor, query, categorySlug, input.Condition, input.MinPrice, input.MaxPrice, sortValue) {
			return ListingPage{}, invalidCursorError("cursor does not match the requested filters")
		}
	}

	var queryValue *string
	if query != "" {
		escaped := escapeILIKE(query)
		queryValue = &escaped
	}
	var categoryID *int64
	if categorySlug != "" {
		category, err := s.categories.GetActiveCategoryBySlug(ctx, categorySlug)
		if errors.Is(err, domain.ErrNotFound) {
			return ListingPage{Items: []domain.Listing{}}, nil
		}
		if err != nil {
			return ListingPage{}, fmt.Errorf("resolve search category: %w", err)
		}
		categoryID = &category.ID
	}

	params := SearchListingsParams{
		Query: queryValue, CategoryID: categoryID, Condition: input.Condition,
		MinPrice: input.MinPrice, MaxPrice: input.MaxPrice,
		Sort: sortValue, Limit: limit + 1,
	}
	if input.Cursor != "" {
		params.CursorID = &cursor.ID
		params.CursorCreatedAt = cursor.CreatedAt
		params.CursorPriceIDR = cursor.PriceIDR
	}
	items, err := s.listings.SearchListings(ctx, params)
	if err != nil {
		return ListingPage{}, fmt.Errorf("search listings: %w", err)
	}
	return pageFromSearchListings(items, limit, query, categorySlug, input.Condition, input.MinPrice, input.MaxPrice, sortValue), nil
}

func (s *CatalogService) ensureOwnerCanMutate(ctx context.Context, sellerID, listingID int64) (domain.Listing, error) {
	listing, err := s.listings.GetListing(ctx, listingID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("load listing for ownership check: %w", err)
	}
	if listing.ModerationStatus == domain.ModerationRemoved {
		return domain.Listing{}, domain.ErrNotFound
	}
	if listing.SellerID != sellerID {
		return domain.Listing{}, domain.ErrForbidden
	}
	return listing, nil
}

func validatePositiveIDs(sellerID, listingID int64) error {
	fields := make(map[string]string)
	if sellerID <= 0 {
		fields["seller_id"] = "Seller is invalid."
	}
	if listingID <= 0 {
		fields["listing_id"] = "Listing is invalid."
	}
	if len(fields) > 0 {
		return &domain.ValidationError{Fields: fields}
	}
	return nil
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultListingPageLimit, nil
	}
	if limit < 1 || limit > MaximumListingPageLimit {
		return 0, newBadRequest("limit", "Limit must be between 1 and 100.")
	}
	return limit, nil
}

func validListingSort(sortValue ListingSort) bool {
	switch sortValue {
	case SortNewest, SortPriceAsc, SortPriceDesc:
		return true
	default:
		return false
	}
}

func pageFromOwnedListings(items []domain.Listing, limit int, status *domain.ListingStatus) ListingPage {
	if items == nil {
		items = []domain.Listing{}
	}
	if len(items) <= limit {
		return ListingPage{Items: items}
	}
	items = items[:limit]
	last := items[len(items)-1]
	statusValue := ""
	if status != nil {
		statusValue = string(*status)
	}
	cursor := encodeOwnedListingsCursor(ownedListingsCursor{
		Version:   cursorVersion,
		Kind:      ownedListingsCursorKind,
		Status:    statusValue,
		UpdatedAt: last.UpdatedAt.UTC(),
		ID:        last.ID,
	})
	return ListingPage{Items: items, NextCursor: &cursor}
}

func pageFromSearchListings(items []domain.Listing, limit int, query, category string, condition *domain.ListingCondition, minPrice, maxPrice *int64, sortValue ListingSort) ListingPage {
	if items == nil {
		items = []domain.Listing{}
	}
	if len(items) <= limit {
		return ListingPage{Items: items}
	}
	items = items[:limit]
	last := items[len(items)-1]
	cursor := searchListingsCursor{
		Version:  cursorVersion,
		Kind:     publicListingsCursorKind,
		Query:    query,
		Category: category,
		Sort:     string(sortValue),
		ID:       last.ID,
	}
	if condition != nil {
		cursor.Condition = string(*condition)
	}
	if minPrice != nil {
		value := *minPrice
		cursor.MinPrice = &value
	}
	if maxPrice != nil {
		value := *maxPrice
		cursor.MaxPrice = &value
	}
	if sortValue == SortNewest {
		value := last.CreatedAt.UTC()
		cursor.CreatedAt = &value
	} else {
		value := last.PriceIDR
		cursor.PriceIDR = &value
	}
	encoded := encodeSearchListingsCursor(cursor)
	return ListingPage{Items: items, NextCursor: &encoded}
}

type ownedListingsCursor struct {
	Version   int       `json:"v"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        int64     `json:"id"`
}

type searchListingsCursor struct {
	Version   int        `json:"v"`
	Kind      string     `json:"kind"`
	Query     string     `json:"q"`
	Category  string     `json:"category"`
	Condition string     `json:"condition"`
	MinPrice  *int64     `json:"min_price"`
	MaxPrice  *int64     `json:"max_price"`
	Sort      string     `json:"sort"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	PriceIDR  *int64     `json:"price_idr,omitempty"`
	ID        int64      `json:"id"`
}

func encodeOwnedListingsCursor(cursor ownedListingsCursor) string {
	return encodeCursor(cursor)
}

func decodeOwnedListingsCursor(raw string) (ownedListingsCursor, error) {
	var cursor ownedListingsCursor
	if err := decodeCursor(raw, &cursor); err != nil || cursor.Version != cursorVersion || cursor.Kind != ownedListingsCursorKind || cursor.ID <= 0 || cursor.UpdatedAt.IsZero() {
		return ownedListingsCursor{}, invalidCursorError("cursor is malformed")
	}
	return cursor, nil
}

func encodeSearchListingsCursor(cursor searchListingsCursor) string {
	return encodeCursor(cursor)
}

func decodeSearchListingsCursor(raw string) (searchListingsCursor, error) {
	var cursor searchListingsCursor
	if err := decodeCursor(raw, &cursor); err != nil {
		return searchListingsCursor{}, invalidCursorError("cursor is malformed")
	}
	if cursor.Version != cursorVersion || cursor.Kind != publicListingsCursorKind || cursor.ID <= 0 || !validListingSort(ListingSort(cursor.Sort)) {
		return searchListingsCursor{}, invalidCursorError("cursor is malformed")
	}
	switch ListingSort(cursor.Sort) {
	case SortNewest:
		if cursor.CreatedAt == nil || cursor.CreatedAt.IsZero() || cursor.PriceIDR != nil {
			return searchListingsCursor{}, invalidCursorError("cursor position is malformed")
		}
	case SortPriceAsc, SortPriceDesc:
		if cursor.PriceIDR == nil || *cursor.PriceIDR < domain.MinimumListingPriceIDR || *cursor.PriceIDR > domain.MaximumListingPriceIDR || cursor.CreatedAt != nil {
			return searchListingsCursor{}, invalidCursorError("cursor position is malformed")
		}
	}
	if cursor.Condition != "" && domain.ValidateListingCondition(domain.ListingCondition(cursor.Condition)) != nil {
		return searchListingsCursor{}, invalidCursorError("cursor filters are malformed")
	}
	if cursor.MinPrice != nil && (domain.ValidateListingPrice(*cursor.MinPrice) != nil || (cursor.MaxPrice != nil && *cursor.MinPrice > *cursor.MaxPrice)) {
		return searchListingsCursor{}, invalidCursorError("cursor filters are malformed")
	}
	if cursor.MaxPrice != nil && domain.ValidateListingPrice(*cursor.MaxPrice) != nil {
		return searchListingsCursor{}, invalidCursorError("cursor filters are malformed")
	}
	if len([]rune(cursor.Query)) > 100 || len([]rune(cursor.Category)) > domain.MaximumCategorySlugLength {
		return searchListingsCursor{}, invalidCursorError("cursor filters are malformed")
	}
	if cursor.Category != "" && domain.ValidateCategorySlug(cursor.Category) != nil {
		return searchListingsCursor{}, invalidCursorError("cursor filters are malformed")
	}
	return cursor, nil
}

func cursorMatchesSearch(cursor searchListingsCursor, query, category string, condition *domain.ListingCondition, minPrice, maxPrice *int64, sortValue ListingSort) bool {
	conditionValue := ""
	if condition != nil {
		conditionValue = string(*condition)
	}
	return cursor.Query == query && cursor.Category == category && cursor.Condition == conditionValue && equalInt64Pointer(cursor.MinPrice, minPrice) && equalInt64Pointer(cursor.MaxPrice, maxPrice) && cursor.Sort == string(sortValue)
}

func equalInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func encodeCursor(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeCursor(raw string, destination any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("cursor has multiple JSON values")
		}
		return err
	}
	return nil
}

func escapeILIKE(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func newBadRequest(field, message string) error {
	return &BadRequestError{
		Code:    "bad_request",
		Message: "The request was invalid.",
		Fields:  map[string]string{field: message},
		Err:     ErrBadRequest,
	}
}

func invalidCursorError(reason string) error {
	return &BadRequestError{
		Code:    "invalid_cursor",
		Message: "The cursor is invalid.",
		Fields:  map[string]string{"cursor": "Cursor is invalid."},
		Err:     errors.New(reason),
	}
}

func mergeValidation(fields map[string]string, err error) {
	if err == nil {
		return
	}
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		for field, message := range validation.Fields {
			fields[field] = message
		}
		return
	}
	fields["listing"] = "Listing is invalid."
}
