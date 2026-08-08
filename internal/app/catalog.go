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
	"unicode/utf8"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const (
	defaultCatalogPageLimit = 20
	maximumCatalogPageLimit = 100
	cursorVersion           = 1
	cursorKindOwned         = "owned_listings"
	cursorKindPublic        = "public_listings"
)

// CategoryRepository defines the category reads required by catalog use cases.
type CategoryRepository interface {
	ListActiveCategories(ctx context.Context) ([]domain.Category, error)
	FindActiveCategoryBySlug(ctx context.Context, slug string) (domain.Category, error)
}

// ListingRepository defines persistence operations required by catalog use cases.
type ListingRepository interface {
	CreateListing(ctx context.Context, params CreateListingParams) (domain.Listing, error)
	GetListing(ctx context.Context, listingID int64) (domain.Listing, error)
	UpdateOwnedListing(ctx context.Context, params UpdateListingParams) (domain.Listing, error)
	UpdateOwnedListingStatus(ctx context.Context, listingID, sellerID int64, status domain.ListingStatus, updatedAt time.Time) (domain.Listing, error)
	ListOwnedListings(ctx context.Context, params OwnedListingQuery) ([]domain.Listing, error)
	SearchListings(ctx context.Context, params SearchListingsQuery) ([]domain.Listing, error)
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
	return &CatalogService{categories: categories, listings: listings, now: now}
}

type CreateListingInput struct {
	Title        string
	Description  string
	PriceIDR     int64
	Quantity     int32
	CategorySlug string
	Condition    domain.ListingCondition
	Negotiable   bool
}

type CreateListingParams struct {
	SellerID    int64
	CategoryID  int64
	Title       string
	Description string
	PriceIDR    int64
	Quantity    int32
	Condition   domain.ListingCondition
	Negotiable  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

type UpdateListingParams struct {
	ListingID   int64
	SellerID    int64
	CategoryID  *int64
	Title       *string
	Description *string
	PriceIDR    *int64
	Quantity    *int32
	Condition   *domain.ListingCondition
	Negotiable  *bool
	UpdatedAt   time.Time
}

type OwnedListingOptions struct {
	Status *domain.ListingStatus
	Cursor string
	Limit  int
}

type OwnedListingQuery struct {
	SellerID        int64
	Status          *domain.ListingStatus
	CursorUpdatedAt *time.Time
	CursorID        *int64
	Limit           int
}

type SearchListingsOptions struct {
	Query        string
	Category     string
	Condition    *domain.ListingCondition
	MinimumPrice *int64
	MaximumPrice *int64
	Sort         ListingSort
	Cursor       string
	Limit        int
}

type ListingSort string

const (
	ListingSortNewest    ListingSort = "newest"
	ListingSortPriceAsc  ListingSort = "price_asc"
	ListingSortPriceDesc ListingSort = "price_desc"
)

type SearchListingsQuery struct {
	Query           *string
	Category        *string
	Condition       *domain.ListingCondition
	MinimumPrice    *int64
	MaximumPrice    *int64
	Sort            ListingSort
	CursorCreatedAt *time.Time
	CursorPriceIDR  *int64
	CursorID        *int64
	Limit           int
}

type ListingPage struct {
	Items      []domain.Listing
	NextCursor *string
}

func (s *CatalogService) ListCategories(ctx context.Context) ([]domain.Category, error) {
	if s.categories == nil {
		return nil, errors.New("category repository is not configured")
	}
	return s.categories.ListActiveCategories(ctx)
}

func (s *CatalogService) CreateListing(ctx context.Context, sellerID int64, input CreateListingInput) (domain.Listing, error) {
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.CategorySlug = domain.NormalizeCategorySlug(input.CategorySlug)
	if err := validateCreateInput(input); err != nil {
		return domain.Listing{}, err
	}

	category, err := s.categories.FindActiveCategoryBySlug(ctx, input.CategorySlug)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Listing{}, domain.NewValidationError(map[string]string{"category_slug": "must identify an active category"})
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("find active category: %w", err)
	}

	now := s.now().UTC()
	return s.listings.CreateListing(ctx, CreateListingParams{
		SellerID: sellerID, CategoryID: category.ID, Title: input.Title, Description: input.Description,
		PriceIDR: input.PriceIDR, Quantity: input.Quantity, Condition: input.Condition,
		Negotiable: input.Negotiable, CreatedAt: now, UpdatedAt: now,
	})
}

func (s *CatalogService) UpdateListing(ctx context.Context, sellerID, listingID int64, input UpdateListingInput) (domain.Listing, error) {
	if s.categories == nil || s.listings == nil {
		return domain.Listing{}, errors.New("catalog repositories are not configured")
	}
	if !input.hasUpdates() {
		return domain.Listing{}, domain.NewValidationError(map[string]string{"body": "must include at least one listing field"})
	}
	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		input.Title = &value
	}
	if input.CategorySlug != nil {
		value := domain.NormalizeCategorySlug(*input.CategorySlug)
		input.CategorySlug = &value
	}
	if err := validateUpdateInput(input); err != nil {
		return domain.Listing{}, err
	}

	existing, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return domain.Listing{}, err
	}
	if existing.ModerationStatus == domain.ModerationStatusRemoved {
		return domain.Listing{}, domain.ErrNotFound
	}
	if existing.SellerID != sellerID {
		return domain.Listing{}, domain.ErrForbidden
	}

	var categoryID *int64
	if input.CategorySlug != nil {
		category, categoryErr := s.categories.FindActiveCategoryBySlug(ctx, *input.CategorySlug)
		if errors.Is(categoryErr, domain.ErrNotFound) {
			return domain.Listing{}, domain.NewValidationError(map[string]string{"category_slug": "must identify an active category"})
		}
		if categoryErr != nil {
			return domain.Listing{}, fmt.Errorf("find active category: %w", categoryErr)
		}
		categoryID = &category.ID
	}

	return s.listings.UpdateOwnedListing(ctx, UpdateListingParams{
		ListingID: listingID, SellerID: sellerID, CategoryID: categoryID, Title: input.Title,
		Description: input.Description, PriceIDR: input.PriceIDR, Quantity: input.Quantity,
		Condition: input.Condition, Negotiable: input.Negotiable, UpdatedAt: s.now().UTC(),
	})
}

func (s *CatalogService) ChangeListingStatus(ctx context.Context, sellerID, listingID int64, status domain.ListingStatus) (domain.Listing, error) {
	if s.listings == nil {
		return domain.Listing{}, errors.New("listing repository is not configured")
	}
	if err := domain.ValidateListingStatus(status); err != nil {
		return domain.Listing{}, err
	}
	existing, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return domain.Listing{}, err
	}
	if existing.ModerationStatus == domain.ModerationStatusRemoved {
		return domain.Listing{}, domain.ErrNotFound
	}
	if existing.SellerID != sellerID {
		return domain.Listing{}, domain.ErrForbidden
	}
	if existing.Status == status {
		return existing, nil
	}
	if !domain.CanTransitionListingStatus(existing.Status, status) {
		return domain.Listing{}, fmt.Errorf("%w: %s cannot transition to %s", domain.ErrConflict, existing.Status, status)
	}
	return s.listings.UpdateOwnedListingStatus(ctx, listingID, sellerID, status, s.now().UTC())
}

func (s *CatalogService) ListOwnedListings(ctx context.Context, sellerID int64, options OwnedListingOptions) (ListingPage, error) {
	if s.listings == nil {
		return ListingPage{}, errors.New("listing repository is not configured")
	}
	limit, err := validatePageLimit(options.Limit)
	if err != nil {
		return ListingPage{}, err
	}
	if options.Status != nil && domain.ValidateListingStatus(*options.Status) != nil {
		return ListingPage{}, domain.NewQueryError(map[string]string{"status": "must be active, reserved, sold, or archived"})
	}
	cursor, err := decodeOwnedCursor(options.Cursor, options.Status)
	if err != nil {
		return ListingPage{}, err
	}
	rows, err := s.listings.ListOwnedListings(ctx, OwnedListingQuery{
		SellerID: sellerID, Status: options.Status, CursorUpdatedAt: cursor.updatedAt, CursorID: cursor.id, Limit: limit + 1,
	})
	if err != nil {
		return ListingPage{}, err
	}
	return ownedListingPage(rows, limit, options.Status)
}

func (s *CatalogService) GetListing(ctx context.Context, listingID int64, viewerID *int64) (domain.Listing, error) {
	if s.listings == nil {
		return domain.Listing{}, errors.New("listing repository is not configured")
	}
	listing, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return domain.Listing{}, err
	}
	if listing.ModerationStatus == domain.ModerationStatusRemoved {
		return domain.Listing{}, domain.ErrNotFound
	}
	if listing.Status == domain.ListingStatusArchived && (viewerID == nil || *viewerID != listing.SellerID) {
		return domain.Listing{}, domain.ErrNotFound
	}
	return listing, nil
}

func (s *CatalogService) SearchListings(ctx context.Context, options SearchListingsOptions) (ListingPage, error) {
	if s.listings == nil {
		return ListingPage{}, errors.New("listing repository is not configured")
	}
	options.Query = strings.TrimSpace(options.Query)
	options.Category = domain.NormalizeCategorySlug(options.Category)
	limit, err := validateSearchOptions(options)
	if err != nil {
		return ListingPage{}, err
	}
	cursor, err := decodePublicCursor(options.Cursor, options)
	if err != nil {
		return ListingPage{}, err
	}
	query := optionalString(escapeILIKE(options.Query))
	category := optionalString(options.Category)
	rows, err := s.listings.SearchListings(ctx, SearchListingsQuery{
		Query: query, Category: category, Condition: options.Condition, MinimumPrice: options.MinimumPrice,
		MaximumPrice: options.MaximumPrice, Sort: normalizedSort(options.Sort), CursorCreatedAt: cursor.createdAt,
		CursorPriceIDR: cursor.priceIDR, CursorID: cursor.id, Limit: limit + 1,
	})
	if err != nil {
		return ListingPage{}, err
	}
	return publicListingPage(rows, limit, options)
}

func validateCreateInput(input CreateListingInput) error {
	fields := map[string]string{}
	collectValidation(fields, domain.ValidateListingTitle(input.Title))
	collectValidation(fields, domain.ValidateDescription(input.Description))
	collectValidation(fields, domain.ValidatePriceIDR(input.PriceIDR))
	collectValidation(fields, domain.ValidateQuantity(input.Quantity))
	collectValidation(fields, domain.ValidateListingCondition(input.Condition))
	if input.CategorySlug == "" {
		fields["category_slug"] = "must not be blank"
	} else if utf8.RuneCountInString(input.CategorySlug) > domain.MaximumCategorySlugLength {
		fields["category_slug"] = fmt.Sprintf("must be at most %d characters", domain.MaximumCategorySlugLength)
	}
	return domain.NewValidationError(fields)
}

func validateUpdateInput(input UpdateListingInput) error {
	fields := map[string]string{}
	if input.Title != nil {
		collectValidation(fields, domain.ValidateListingTitle(*input.Title))
	}
	if input.Description != nil {
		collectValidation(fields, domain.ValidateDescription(*input.Description))
	}
	if input.PriceIDR != nil {
		collectValidation(fields, domain.ValidatePriceIDR(*input.PriceIDR))
	}
	if input.Quantity != nil {
		collectValidation(fields, domain.ValidateQuantity(*input.Quantity))
	}
	if input.Condition != nil {
		collectValidation(fields, domain.ValidateListingCondition(*input.Condition))
	}
	if input.CategorySlug != nil {
		if *input.CategorySlug == "" {
			fields["category_slug"] = "must not be blank"
		} else if utf8.RuneCountInString(*input.CategorySlug) > domain.MaximumCategorySlugLength {
			fields["category_slug"] = fmt.Sprintf("must be at most %d characters", domain.MaximumCategorySlugLength)
		}
	}
	return domain.NewValidationError(fields)
}

func collectValidation(fields map[string]string, err error) {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		for field, message := range validation.Fields {
			fields[field] = message
		}
	}
}

func (input UpdateListingInput) hasUpdates() bool {
	return input.Title != nil || input.Description != nil || input.PriceIDR != nil || input.Quantity != nil || input.CategorySlug != nil || input.Condition != nil || input.Negotiable != nil
}

func validatePageLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultCatalogPageLimit, nil
	}
	if limit < 1 || limit > maximumCatalogPageLimit {
		return 0, domain.NewQueryError(map[string]string{"limit": fmt.Sprintf("must be between 1 and %d", maximumCatalogPageLimit)})
	}
	return limit, nil
}

func validateSearchOptions(options SearchListingsOptions) (int, error) {
	limit, err := validatePageLimit(options.Limit)
	if err != nil {
		return 0, err
	}
	fields := map[string]string{}
	if utf8.RuneCountInString(options.Query) > 100 {
		fields["q"] = "must be at most 100 characters"
	}
	if options.Condition != nil && domain.ValidateListingCondition(*options.Condition) != nil {
		fields["condition"] = "must be new or used"
	}
	if options.MinimumPrice != nil && (*options.MinimumPrice < domain.MinimumListingPriceIDR || *options.MinimumPrice > domain.MaximumListingPriceIDR) {
		fields["min_price"] = "is outside the allowed price range"
	}
	if options.MaximumPrice != nil && (*options.MaximumPrice < domain.MinimumListingPriceIDR || *options.MaximumPrice > domain.MaximumListingPriceIDR) {
		fields["max_price"] = "is outside the allowed price range"
	}
	if options.MinimumPrice != nil && options.MaximumPrice != nil && *options.MinimumPrice > *options.MaximumPrice {
		fields["min_price"] = "must not be greater than max_price"
	}
	if !validSort(normalizedSort(options.Sort)) {
		fields["sort"] = "must be newest, price_asc, or price_desc"
	}
	if len(fields) > 0 {
		return 0, domain.NewQueryError(fields)
	}
	return limit, nil
}

func normalizedSort(value ListingSort) ListingSort {
	if value == "" {
		return ListingSortNewest
	}
	return value
}

func validSort(value ListingSort) bool {
	return value == ListingSortNewest || value == ListingSortPriceAsc || value == ListingSortPriceDesc
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func escapeILIKE(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

type ownedCursor struct {
	Version   int     `json:"v"`
	Kind      string  `json:"k"`
	Status    *string `json:"s"`
	UpdatedAt string  `json:"u"`
	ID        int64   `json:"i"`

	updatedAt *time.Time
	id        *int64
}

func decodeOwnedCursor(value string, status *domain.ListingStatus) (ownedCursor, error) {
	if value == "" {
		return ownedCursor{}, nil
	}
	var cursor ownedCursor
	if err := decodeCursor(value, &cursor); err != nil || cursor.Version != cursorVersion || cursor.Kind != cursorKindOwned || cursor.ID < 1 {
		return ownedCursor{}, invalidCursorError()
	}
	expected := ""
	if status != nil {
		expected = string(*status)
	}
	actual := ""
	if cursor.Status != nil {
		actual = *cursor.Status
	}
	if actual != expected {
		return ownedCursor{}, invalidCursorError()
	}
	parsed, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return ownedCursor{}, invalidCursorError()
	}
	cursor.updatedAt = &parsed
	cursor.id = &cursor.ID
	return cursor, nil
}

func ownedListingPage(rows []domain.Listing, limit int, status *domain.ListingStatus) (ListingPage, error) {
	page := ListingPage{Items: rows}
	if len(rows) <= limit {
		return page, nil
	}
	page.Items = rows[:limit]
	last := page.Items[len(page.Items)-1]
	statusValue := ""
	if status != nil {
		statusValue = string(*status)
	}
	cursor, err := encodeCursor(ownedCursor{Version: cursorVersion, Kind: cursorKindOwned, Status: optionalString(statusValue), UpdatedAt: last.UpdatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID})
	if err != nil {
		return ListingPage{}, fmt.Errorf("encode owned listing cursor: %w", err)
	}
	page.NextCursor = &cursor
	return page, nil
}

type publicCursor struct {
	Version      int         `json:"v"`
	Kind         string      `json:"k"`
	Query        string      `json:"q"`
	Category     string      `json:"c"`
	Condition    *string     `json:"o"`
	MinimumPrice *int64      `json:"n"`
	MaximumPrice *int64      `json:"x"`
	Sort         ListingSort `json:"s"`
	CreatedAt    string      `json:"t,omitempty"`
	PriceIDR     *int64      `json:"p,omitempty"`
	ID           int64       `json:"i"`

	createdAt *time.Time
	priceIDR  *int64
	id        *int64
}

func decodePublicCursor(value string, options SearchListingsOptions) (publicCursor, error) {
	if value == "" {
		return publicCursor{}, nil
	}
	var cursor publicCursor
	if err := decodeCursor(value, &cursor); err != nil || cursor.Version != cursorVersion || cursor.Kind != cursorKindPublic || cursor.ID < 1 || !validSort(cursor.Sort) {
		return publicCursor{}, invalidCursorError()
	}
	condition := ""
	if options.Condition != nil {
		condition = string(*options.Condition)
	}
	actualCondition := ""
	if cursor.Condition != nil {
		actualCondition = *cursor.Condition
	}
	if cursor.Query != options.Query || cursor.Category != options.Category || actualCondition != condition || !equalInt64Pointers(cursor.MinimumPrice, options.MinimumPrice) || !equalInt64Pointers(cursor.MaximumPrice, options.MaximumPrice) || cursor.Sort != normalizedSort(options.Sort) {
		return publicCursor{}, invalidCursorError()
	}
	switch cursor.Sort {
	case ListingSortNewest:
		parsed, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
		if err != nil {
			return publicCursor{}, invalidCursorError()
		}
		cursor.createdAt = &parsed
	case ListingSortPriceAsc, ListingSortPriceDesc:
		if cursor.PriceIDR == nil || *cursor.PriceIDR < domain.MinimumListingPriceIDR || *cursor.PriceIDR > domain.MaximumListingPriceIDR {
			return publicCursor{}, invalidCursorError()
		}
		cursor.priceIDR = cursor.PriceIDR
	}
	cursor.id = &cursor.ID
	return cursor, nil
}

func publicListingPage(rows []domain.Listing, limit int, options SearchListingsOptions) (ListingPage, error) {
	page := ListingPage{Items: rows}
	if len(rows) <= limit {
		return page, nil
	}
	page.Items = rows[:limit]
	last := page.Items[len(page.Items)-1]
	sort := normalizedSort(options.Sort)
	cursor := publicCursor{
		Version: cursorVersion, Kind: cursorKindPublic, Query: options.Query, Category: options.Category,
		Condition: listingConditionPointer(options.Condition), MinimumPrice: options.MinimumPrice, MaximumPrice: options.MaximumPrice,
		Sort: sort, ID: last.ID,
	}
	if sort == ListingSortNewest {
		cursor.CreatedAt = last.CreatedAt.UTC().Format(time.RFC3339Nano)
	} else {
		price := last.PriceIDR
		cursor.PriceIDR = &price
	}
	encoded, err := encodeCursor(cursor)
	if err != nil {
		return ListingPage{}, fmt.Errorf("encode public listing cursor: %w", err)
	}
	page.NextCursor = &encoded
	return page, nil
}

func listingConditionPointer(value *domain.ListingCondition) *string {
	if value == nil {
		return nil
	}
	copy := string(*value)
	return &copy
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func encodeCursor(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(value string, destination any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return errors.New("decode cursor")
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("cursor contains multiple JSON values")
		}
		return err
	}
	return nil
}

func invalidCursorError() error {
	return domain.NewQueryError(map[string]string{"cursor": "is invalid or does not match this request"})
}
