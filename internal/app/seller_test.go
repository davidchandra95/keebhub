package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestSellerServiceUpdatesOnlyEffectiveProfileChanges(t *testing.T) {
	t.Parallel()

	location := "Jakarta"
	bio := "Keyboard enthusiast"
	user := domain.User{ID: 42, Status: domain.UserStatusActive, Location: &location, Bio: &bio}
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repository := &sellerRepository{updatedUser: user}
	service := app.NewSellerService(repository, repository, func() time.Time { return now })

	unchanged, err := service.UpdateProfile(context.Background(), user, app.UpdateProfileInput{Location: presentProfileValue(" Jakarta ")})
	if err != nil || unchanged.Location == nil || *unchanged.Location != "Jakarta" || repository.updateCalled {
		t.Fatalf("no-op UpdateProfile() = %+v, %v, write = %v", unchanged, err, repository.updateCalled)
	}

	updated, err := service.UpdateProfile(context.Background(), user, app.UpdateProfileInput{
		Location: presentProfileValue(" Bandung "),
		Bio:      app.ProfileField{Present: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if !repository.updateCalled || repository.update.UserID != 42 || !repository.update.SetLocation || repository.update.Location == nil || *repository.update.Location != "Bandung" || !repository.update.SetBio || repository.update.Bio != nil || repository.update.UpdatedAt != now || updated.ID != user.ID {
		t.Errorf("update result/params = %+v/%+v", updated, repository.update)
	}
}

func TestSellerServiceProfileValidationAndDisabledMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    domain.User
		input   app.UpdateProfileInput
		repoErr error
		wantErr error
	}{
		{name: "unicode over limit", user: activeSeller(), input: app.UpdateProfileInput{Bio: presentProfileValue(strings.Repeat("界", domain.MaximumProfileBioLength+1))}, wantErr: &domain.ValidationError{}},
		{name: "disabled before write", user: domain.User{ID: 42, Status: domain.UserStatusDisabled}, input: app.UpdateProfileInput{Location: presentProfileValue("Jakarta")}, wantErr: domain.ErrUserDisabled},
		{name: "disabled during write", user: activeSeller(), input: app.UpdateProfileInput{Location: presentProfileValue("Jakarta")}, repoErr: domain.ErrUserDisabled, wantErr: domain.ErrUserDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repository := &sellerRepository{updateErr: tt.repoErr}
			service := app.NewSellerService(repository, repository, time.Now)
			_, err := service.UpdateProfile(context.Background(), tt.user, tt.input)
			var validation *domain.ValidationError
			if _, ok := tt.wantErr.(*domain.ValidationError); ok {
				if !errors.As(err, &validation) || repository.updateCalled {
					t.Fatalf("UpdateProfile() error/write = %v/%v", err, repository.updateCalled)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("UpdateProfile() error = %v, want %v", err, tt.wantErr)
			}
			if tt.name == "disabled before write" && repository.updateCalled {
				t.Error("disabled user profile update wrote to repository")
			}
		})
	}
}

func TestSellerServiceCatalogNormalizesAndBindsCursors(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repository := &sellerRepository{
		profiles: map[string]domain.SellerProfile{
			"seller-one": {User: domain.PublicUser{ID: 42, Handle: "seller-one"}},
			"seller-two": {User: domain.PublicUser{ID: 43, Handle: "seller-two"}},
		},
		listingRows: []domain.Listing{
			{ID: 11, Status: domain.ListingStatusActive, UpdatedAt: updatedAt},
			{ID: 10, Status: domain.ListingStatusReserved, UpdatedAt: updatedAt.Add(-time.Minute)},
		},
	}
	service := app.NewSellerService(repository, repository, time.Now)
	page, err := service.ListSellerListings(context.Background(), "seller-one", app.SellerListingOptions{Category: " keyboard ", Limit: 1})
	if err != nil {
		t.Fatalf("ListSellerListings() error = %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == nil || repository.listingQuery.Category == nil || *repository.listingQuery.Category != "keyboard" || strings.Join(repository.listingQuery.Statuses, ",") != "active,reserved" || repository.listingQuery.Limit != 2 {
		t.Fatalf("page/query = %+v/%+v", page, repository.listingQuery)
	}

	repository.listingRows = nil
	_, err = service.ListSellerListings(context.Background(), "seller-one", app.SellerListingOptions{Category: "keyboard", Limit: 10, Cursor: *page.NextCursor})
	if err != nil || repository.listingQuery.CursorStatusRank == nil || *repository.listingQuery.CursorStatusRank != 0 || repository.listingQuery.CursorID == nil || *repository.listingQuery.CursorID != 11 {
		t.Fatalf("next page error/query = %v/%+v", err, repository.listingQuery)
	}
	if _, err := service.ListSellerListings(context.Background(), "seller-two", app.SellerListingOptions{Category: "keyboard", Cursor: *page.NextCursor}); err == nil {
		t.Fatal("seller cursor was accepted for a different seller")
	}
	if _, err := service.ListSellerListings(context.Background(), "seller-one", app.SellerListingOptions{Category: "keycaps", Cursor: *page.NextCursor}); err == nil {
		t.Fatal("seller cursor was accepted for another category")
	}
}

func TestSellerServiceCatalogValidationAndPublicProfile(t *testing.T) {
	t.Parallel()

	repository := &sellerRepository{profiles: map[string]domain.SellerProfile{
		"disabled-seller": {User: domain.PublicUser{ID: 42, Handle: "disabled-seller"}, ActiveListingCount: 0},
	}}
	service := app.NewSellerService(repository, repository, time.Now)
	profile, err := service.GetSellerProfile(context.Background(), "disabled-seller")
	if err != nil || profile.ActiveListingCount != 0 {
		t.Fatalf("GetSellerProfile() = %+v, %v", profile, err)
	}
	for _, options := range []app.SellerListingOptions{
		{Status: listingStatusPointer(domain.ListingStatusArchived)},
		{Category: strings.Repeat("a", domain.MaximumCategorySlugLength+1)},
		{Limit: 101},
		{Cursor: "not-a-cursor"},
	} {
		_, err := service.ListSellerListings(context.Background(), "disabled-seller", options)
		var query *domain.QueryError
		if !errors.As(err, &query) {
			t.Errorf("ListSellerListings(%+v) error = %v, want query error", options, err)
		}
	}
	if _, err := service.GetSellerProfile(context.Background(), "Seller"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("malformed handle error = %v", err)
	}
}

type sellerRepository struct {
	profiles     map[string]domain.SellerProfile
	updatedUser  domain.User
	update       app.UpdateProfileParams
	updateErr    error
	updateCalled bool
	listingRows  []domain.Listing
	listingQuery app.SellerListingQuery
}

func (r *sellerRepository) UpdateProfile(_ context.Context, params app.UpdateProfileParams) (domain.User, error) {
	r.updateCalled = true
	r.update = params
	if r.updateErr != nil {
		return domain.User{}, r.updateErr
	}
	if r.updatedUser.ID == 0 {
		r.updatedUser = domain.User{ID: params.UserID, Status: domain.UserStatusActive, Location: params.Location, Bio: params.Bio, UpdatedAt: params.UpdatedAt}
	}
	return r.updatedUser, nil
}

func (r *sellerRepository) GetSellerProfile(_ context.Context, handle string) (domain.SellerProfile, error) {
	profile, ok := r.profiles[handle]
	if !ok {
		return domain.SellerProfile{}, domain.ErrNotFound
	}
	return profile, nil
}

func (r *sellerRepository) ListSellerListings(_ context.Context, query app.SellerListingQuery) ([]domain.Listing, error) {
	r.listingQuery = query
	return r.listingRows, nil
}

func activeSeller() domain.User {
	return domain.User{ID: 42, Status: domain.UserStatusActive}
}

func presentProfileValue(value string) app.ProfileField {
	return app.ProfileField{Present: true, Value: &value}
}

func listingStatusPointer(value domain.ListingStatus) *domain.ListingStatus { return &value }
