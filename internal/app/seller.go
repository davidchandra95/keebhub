package app

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const cursorKindSeller = "seller_listings"

// SellerRepository defines the profile persistence required by seller use cases.
type SellerRepository interface {
	UpdateProfile(ctx context.Context, params UpdateProfileParams) (domain.User, error)
	GetSellerProfile(ctx context.Context, handle string) (domain.SellerProfile, error)
}

// SellerCatalogRepository defines seller-scoped listing reads.
type SellerCatalogRepository interface {
	ListSellerListings(ctx context.Context, query SellerListingQuery) ([]domain.Listing, error)
}

// ProfileField preserves the difference between an omitted field and an explicit null.
type ProfileField struct {
	Present bool
	Value   *string
}

type UpdateProfileInput struct {
	Location ProfileField
	Bio      ProfileField
}

type UpdateProfileParams struct {
	UserID      int64
	SetLocation bool
	Location    *string
	SetBio      bool
	Bio         *string
	UpdatedAt   time.Time
}

type SellerListingOptions struct {
	Status   *domain.ListingStatus
	Category string
	Cursor   string
	Limit    int
}

type SellerListingQuery struct {
	SellerID         int64
	Statuses         []string
	Category         *string
	CursorStatusRank *int32
	CursorUpdatedAt  *time.Time
	CursorID         *int64
	Limit            int
}

type SellerService struct {
	repository SellerRepository
	catalog    SellerCatalogRepository
	now        func() time.Time
}

func NewSellerService(repository SellerRepository, catalog SellerCatalogRepository, now func() time.Time) *SellerService {
	if now == nil {
		now = time.Now
	}
	return &SellerService{repository: repository, catalog: catalog, now: now}
}

func (s *SellerService) UpdateProfile(ctx context.Context, user domain.User, input UpdateProfileInput) (domain.User, error) {
	if s.repository == nil {
		return domain.User{}, errors.New("seller repository is not configured")
	}
	if user.Status != domain.UserStatusActive {
		return domain.User{}, domain.ErrUserDisabled
	}
	if !input.Location.Present && !input.Bio.Present {
		return domain.User{}, domain.NewValidationError(map[string]string{"body": "must include location or bio"})
	}

	location := user.Location
	bio := user.Bio
	fields := map[string]string{}
	if input.Location.Present {
		var err error
		location, err = domain.NormalizeProfileLocation(input.Location.Value)
		collectValidation(fields, err)
	}
	if input.Bio.Present {
		var err error
		bio, err = domain.NormalizeProfileBio(input.Bio.Value)
		collectValidation(fields, err)
	}
	if err := domain.NewValidationError(fields); err != nil {
		return domain.User{}, err
	}

	locationChanged := input.Location.Present && !equalOptionalStrings(location, user.Location)
	bioChanged := input.Bio.Present && !equalOptionalStrings(bio, user.Bio)
	if !locationChanged && !bioChanged {
		return user, nil
	}

	return s.repository.UpdateProfile(ctx, UpdateProfileParams{
		UserID: user.ID, SetLocation: input.Location.Present, Location: location,
		SetBio: input.Bio.Present, Bio: bio, UpdatedAt: s.now().UTC(),
	})
}

func (s *SellerService) GetSellerProfile(ctx context.Context, handle string) (domain.SellerProfile, error) {
	if s.repository == nil {
		return domain.SellerProfile{}, errors.New("seller repository is not configured")
	}
	if !domain.IsCanonicalHandle(handle) {
		return domain.SellerProfile{}, domain.ErrNotFound
	}
	return s.repository.GetSellerProfile(ctx, handle)
}

func (s *SellerService) ListSellerListings(ctx context.Context, handle string, options SellerListingOptions) (ListingPage, error) {
	if s.repository == nil || s.catalog == nil {
		return ListingPage{}, errors.New("seller repositories are not configured")
	}
	if !domain.IsCanonicalHandle(handle) {
		return ListingPage{}, domain.ErrNotFound
	}
	options.Category = domain.NormalizeCategorySlug(options.Category)
	limit, err := validatePageLimit(options.Limit)
	if err != nil {
		return ListingPage{}, err
	}
	if utf8.RuneCountInString(options.Category) > domain.MaximumCategorySlugLength {
		return ListingPage{}, domain.NewQueryError(map[string]string{"category": fmt.Sprintf("must be at most %d characters", domain.MaximumCategorySlugLength)})
	}
	if options.Status != nil && !sellerCatalogStatus(*options.Status) {
		return ListingPage{}, domain.NewQueryError(map[string]string{"status": "must be active, reserved, or sold"})
	}

	profile, err := s.GetSellerProfile(ctx, handle)
	if err != nil {
		return ListingPage{}, err
	}
	cursor, err := decodeSellerCursor(options.Cursor, profile.User.ID, options.Status, options.Category)
	if err != nil {
		return ListingPage{}, err
	}
	rows, err := s.catalog.ListSellerListings(ctx, SellerListingQuery{
		SellerID: profile.User.ID, Statuses: sellerCatalogStatuses(options.Status), Category: optionalString(options.Category),
		CursorStatusRank: cursor.rank, CursorUpdatedAt: cursor.updatedAt, CursorID: cursor.id, Limit: limit + 1,
	})
	if err != nil {
		return ListingPage{}, err
	}
	return sellerListingPage(rows, limit, profile.User.ID, options.Status, options.Category)
}

func equalOptionalStrings(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sellerCatalogStatus(status domain.ListingStatus) bool {
	return status == domain.ListingStatusActive || status == domain.ListingStatusReserved || status == domain.ListingStatusSold
}

func sellerCatalogStatuses(status *domain.ListingStatus) []string {
	if status == nil {
		return []string{string(domain.ListingStatusActive), string(domain.ListingStatusReserved)}
	}
	return []string{string(*status)}
}

type sellerCursor struct {
	Version    int     `json:"v"`
	Kind       string  `json:"k"`
	SellerID   int64   `json:"s"`
	Status     *string `json:"t"`
	Category   string  `json:"c"`
	StatusRank int32   `json:"r"`
	UpdatedAt  string  `json:"u"`
	ID         int64   `json:"i"`

	rank      *int32
	updatedAt *time.Time
	id        *int64
}

func decodeSellerCursor(value string, sellerID int64, status *domain.ListingStatus, category string) (sellerCursor, error) {
	if value == "" {
		return sellerCursor{}, nil
	}
	var cursor sellerCursor
	if err := decodeCursor(value, &cursor); err != nil || cursor.Version != cursorVersion || cursor.Kind != cursorKindSeller || cursor.SellerID != sellerID || cursor.ID < 1 {
		return sellerCursor{}, invalidCursorError()
	}
	expectedStatus := ""
	if status != nil {
		expectedStatus = string(*status)
	}
	actualStatus := ""
	if cursor.Status != nil {
		actualStatus = *cursor.Status
	}
	if actualStatus != expectedStatus || (status == nil && cursor.Status != nil) || cursor.Category != category || !validSellerCursorRank(cursor.StatusRank, status) {
		return sellerCursor{}, invalidCursorError()
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return sellerCursor{}, invalidCursorError()
	}
	cursor.rank = &cursor.StatusRank
	cursor.updatedAt = &updatedAt
	cursor.id = &cursor.ID
	return cursor, nil
}

func validSellerCursorRank(rank int32, status *domain.ListingStatus) bool {
	if status == nil {
		return rank == 0 || rank == 1
	}
	return rank == sellerStatusRank(*status)
}

func sellerListingPage(rows []domain.Listing, limit int, sellerID int64, status *domain.ListingStatus, category string) (ListingPage, error) {
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
	cursor, err := encodeCursor(sellerCursor{
		Version: cursorVersion, Kind: cursorKindSeller, SellerID: sellerID, Status: optionalString(statusValue), Category: category,
		StatusRank: sellerStatusRank(last.Status), UpdatedAt: last.UpdatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID,
	})
	if err != nil {
		return ListingPage{}, fmt.Errorf("encode seller listing cursor: %w", err)
	}
	page.NextCursor = &cursor
	return page, nil
}

func sellerStatusRank(status domain.ListingStatus) int32 {
	switch status {
	case domain.ListingStatusActive:
		return 0
	case domain.ListingStatusReserved:
		return 1
	default:
		return 2
	}
}
