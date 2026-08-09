package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// SellerService describes profile and public seller catalog use cases consumed by HTTP.
type SellerService interface {
	UpdateProfile(ctx context.Context, user domain.User, input app.UpdateProfileInput) (domain.User, error)
	GetSellerProfile(ctx context.Context, handle string) (domain.SellerProfile, error)
	ListSellerListings(ctx context.Context, handle string, options app.SellerListingOptions) (app.ListingPage, error)
}

type sellerHandlers struct {
	seller SellerService
	logger *zap.Logger
}

func newSellerHandlers(cfg Config) sellerHandlers {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return sellerHandlers{seller: cfg.Seller, logger: logger}
}

func (h sellerHandlers) updateProfile(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	var request updateProfileRequest
	if err := decodeJSONRequest(c, &request); err != nil {
		return err
	}
	input, changedFields, err := request.input()
	if err != nil {
		return err
	}
	updated, err := h.seller.UpdateProfile(c.Request().Context(), user, input)
	if err != nil {
		return sellerError(err)
	}
	h.logger.Info("Profile updated", zap.String("request_id", RequestID(c)), zap.Int64("user_id", user.ID), zap.Strings("changed_fields", changedFields))
	return c.JSON(http.StatusOK, currentUserResponse{User: userResponseFromDomain(updated)})
}

func (h sellerHandlers) getSellerProfile(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	profile, err := h.seller.GetSellerProfile(c.Request().Context(), c.Param("handle"))
	if err != nil {
		return sellerError(err)
	}
	return c.JSON(http.StatusOK, sellerProfileEnvelope{User: sellerProfileResponseFromDomain(profile)})
}

func (h sellerHandlers) listSellerListings(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	options, err := sellerListingOptions(c)
	if err != nil {
		return err
	}
	page, err := h.seller.ListSellerListings(c.Request().Context(), c.Param("handle"), options)
	if err != nil {
		return sellerError(err)
	}
	return c.JSON(http.StatusOK, listingPageResponseFromDomain(page))
}

func (h sellerHandlers) available() error {
	if h.seller != nil {
		return nil
	}
	return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(errors.New("seller service is not configured"))
}

type updateProfileRequest struct {
	Location jsonField[string] `json:"location"`
	Bio      jsonField[string] `json:"bio"`
}

func (r updateProfileRequest) input() (app.UpdateProfileInput, []string, error) {
	fields := map[string]jsonFieldMarker{"location": r.Location, "bio": r.Bio}
	if !hasPresentField(fields) {
		return app.UpdateProfileInput{}, nil, malformedJSONBodyError()
	}
	input := app.UpdateProfileInput{Location: profileField(r.Location), Bio: profileField(r.Bio)}
	changed := make([]string, 0, 2)
	if r.Location.present {
		changed = append(changed, "location")
	}
	if r.Bio.present {
		changed = append(changed, "bio")
	}
	return input, changed, nil
}

func profileField(field jsonField[string]) app.ProfileField {
	value := (*string)(nil)
	if field.present && !field.null {
		value = fieldPointer(field.value)
	}
	return app.ProfileField{Present: field.present, Value: value}
}

type sellerProfileEnvelope struct {
	User sellerProfileResponse `json:"user"`
}

type sellerProfileResponse struct {
	ID                 string  `json:"id"`
	Handle             string  `json:"handle"`
	DisplayName        string  `json:"display_name"`
	AvatarURL          *string `json:"avatar_url"`
	Location           *string `json:"location"`
	Bio                *string `json:"bio"`
	CreatedAt          string  `json:"created_at"`
	ActiveListingCount int64   `json:"active_listing_count"`
}

func sellerProfileResponseFromDomain(profile domain.SellerProfile) sellerProfileResponse {
	return sellerProfileResponse{
		ID: listingIDString(profile.User.ID), Handle: profile.User.Handle, DisplayName: profile.User.DisplayName,
		AvatarURL: profile.User.AvatarURL, Location: profile.User.Location, Bio: profile.User.Bio,
		CreatedAt: formatTimestamp(profile.CreatedAt), ActiveListingCount: profile.ActiveListingCount,
	}
}

func sellerListingOptions(c *echo.Context) (app.SellerListingOptions, error) {
	limit, err := optionalQueryInt(c, "limit")
	if err != nil {
		return app.SellerListingOptions{}, err
	}
	var status *domain.ListingStatus
	if values, present := c.QueryParams()["status"]; present && len(values) > 0 {
		parsed := domain.ListingStatus(values[0])
		status = &parsed
	}
	return app.SellerListingOptions{Status: status, Category: c.QueryParam("category"), Cursor: c.QueryParam("cursor"), Limit: limit}, nil
}

func sellerError(err error) error {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Request validation failed.", Fields: validation.Fields, Err: err}
	}
	var query *domain.QueryError
	if errors.As(err, &query) {
		return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed.", Fields: query.Fields, Err: err}
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return &Error{Status: http.StatusNotFound, Code: "seller_not_found", Message: "Seller was not found.", Err: err}
	case errors.Is(err, domain.ErrUserDisabled):
		return &Error{Status: http.StatusForbidden, Code: "account_disabled", Message: "This account is disabled.", Err: err}
	default:
		return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(err)
	}
}
