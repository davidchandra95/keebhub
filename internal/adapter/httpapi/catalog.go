package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const maximumListingRequestBytes = 32 << 10

// CatalogService describes catalog use cases consumed by the HTTP adapter.
type CatalogService interface {
	ListCategories(ctx context.Context) ([]domain.Category, error)
	CreateListing(ctx context.Context, sellerID int64, input app.CreateListingInput) (domain.Listing, error)
	UpdateListing(ctx context.Context, sellerID, listingID int64, input app.UpdateListingInput) (domain.Listing, error)
	ChangeListingStatus(ctx context.Context, sellerID, listingID int64, status domain.ListingStatus) (domain.Listing, error)
	ListOwnedListings(ctx context.Context, sellerID int64, options app.OwnedListingOptions) (app.ListingPage, error)
	GetListing(ctx context.Context, listingID int64, viewerID *int64) (domain.Listing, error)
	SearchListings(ctx context.Context, options app.SearchListingsOptions) (app.ListingPage, error)
}

type catalogHandlers struct {
	catalog CatalogService
	logger  *zap.Logger
}

func newCatalogHandlers(cfg Config) catalogHandlers {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return catalogHandlers{catalog: cfg.Catalog, logger: logger}
}

func (h catalogHandlers) listCategories(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	categories, err := h.catalog.ListCategories(c.Request().Context())
	if err != nil {
		return catalogError(err)
	}
	items := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryResponseFromDomain(category))
	}
	return c.JSON(http.StatusOK, categoriesResponse{Items: items})
}

func (h catalogHandlers) searchListings(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	options, err := searchListingOptions(c)
	if err != nil {
		return err
	}
	page, err := h.catalog.SearchListings(c.Request().Context(), options)
	if err != nil {
		return catalogError(err)
	}
	return c.JSON(http.StatusOK, listingPageResponseFromDomain(page))
}

func (h catalogHandlers) createListing(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	var request createListingRequest
	if err := decodeListingRequest(c, &request); err != nil {
		return err
	}
	input, err := request.input()
	if err != nil {
		return catalogError(err)
	}
	listing, err := h.catalog.CreateListing(c.Request().Context(), user.ID, input)
	if err != nil {
		return catalogError(err)
	}
	h.logger.Info("Listing created", zap.String("request_id", RequestID(c)), zap.Int64("seller_id", user.ID), zap.Int64("listing_id", listing.ID))
	return c.JSON(http.StatusCreated, listingEnvelope{Listing: listingResponseFromDomain(listing)})
}

func (h catalogHandlers) updateListing(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	listingID, err := listingIDFromPath(c)
	if err != nil {
		return err
	}
	var request updateListingRequest
	if err := decodeListingRequest(c, &request); err != nil {
		return err
	}
	input, err := request.input()
	if err != nil {
		return catalogError(err)
	}
	listing, err := h.catalog.UpdateListing(c.Request().Context(), user.ID, listingID, input)
	if err != nil {
		return catalogError(err)
	}
	h.logger.Info("Listing updated", zap.String("request_id", RequestID(c)), zap.Int64("seller_id", user.ID), zap.Int64("listing_id", listing.ID))
	return c.JSON(http.StatusOK, listingEnvelope{Listing: listingResponseFromDomain(listing)})
}

func (h catalogHandlers) changeListingStatus(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	listingID, err := listingIDFromPath(c)
	if err != nil {
		return err
	}
	var request changeListingStatusRequest
	if err := decodeListingRequest(c, &request); err != nil {
		return err
	}
	status, err := request.status()
	if err != nil {
		return catalogError(err)
	}
	existing, err := h.catalog.GetListing(c.Request().Context(), listingID, &user.ID)
	if err != nil {
		return catalogError(err)
	}
	listing, err := h.catalog.ChangeListingStatus(c.Request().Context(), user.ID, listingID, status)
	if err != nil {
		return catalogError(err)
	}
	h.logger.Info("Listing status changed",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", user.ID),
		zap.Int64("listing_id", listing.ID),
		zap.String("old_status", string(existing.Status)),
		zap.String("new_status", string(listing.Status)),
	)
	return c.JSON(http.StatusOK, listingEnvelope{Listing: listingResponseFromDomain(listing)})
}

func (h catalogHandlers) listOwnedListings(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	options, err := ownedListingOptions(c)
	if err != nil {
		return err
	}
	page, err := h.catalog.ListOwnedListings(c.Request().Context(), user.ID, options)
	if err != nil {
		return catalogError(err)
	}
	return c.JSON(http.StatusOK, listingPageResponseFromDomain(page))
}

func (h catalogHandlers) getListing(c *echo.Context) error {
	if err := h.available(); err != nil {
		return err
	}
	listingID, err := listingIDFromPath(c)
	if err != nil {
		return err
	}
	var viewerID *int64
	if user, ok := CurrentUser(c); ok {
		viewerID = &user.ID
	}
	listing, err := h.catalog.GetListing(c.Request().Context(), listingID, viewerID)
	if err != nil {
		return catalogError(err)
	}
	return c.JSON(http.StatusOK, listingEnvelope{Listing: listingResponseFromDomain(listing)})
}

func (h catalogHandlers) available() error {
	if h.catalog != nil {
		return nil
	}
	return &Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred.", Err: errors.New("catalog service is not configured")}
}

type categoryResponse struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type categoriesResponse struct {
	Items []categoryResponse `json:"items"`
}

type publicUserResponse struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Location    *string `json:"location"`
	Bio         *string `json:"bio"`
}

type listingResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	PriceIDR    int64              `json:"price_idr"`
	Quantity    int32              `json:"quantity"`
	Category    categoryResponse   `json:"category"`
	Condition   string             `json:"condition"`
	Status      string             `json:"status"`
	Negotiable  bool               `json:"negotiable"`
	Seller      publicUserResponse `json:"seller"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

type listingEnvelope struct {
	Listing listingResponse `json:"listing"`
}

type listingPageResponse struct {
	Items      []listingResponse `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

func categoryResponseFromDomain(category domain.Category) categoryResponse {
	return categoryResponse{ID: strconv.FormatInt(category.ID, 10), Slug: category.Slug, Name: category.Name}
}

func listingResponseFromDomain(listing domain.Listing) listingResponse {
	return listingResponse{
		ID: listingIDString(listing.ID), Title: listing.Title, Description: listing.Description,
		PriceIDR: listing.PriceIDR, Quantity: listing.Quantity, Category: categoryResponseFromDomain(listing.Category),
		Condition: string(listing.Condition), Status: string(listing.Status), Negotiable: listing.Negotiable,
		Seller:    publicUserResponse{ID: listingIDString(listing.Seller.ID), Handle: listing.Seller.Handle, DisplayName: listing.Seller.DisplayName, AvatarURL: listing.Seller.AvatarURL, Location: listing.Seller.Location, Bio: listing.Seller.Bio},
		CreatedAt: formatTimestamp(listing.CreatedAt), UpdatedAt: formatTimestamp(listing.UpdatedAt),
	}
}

func listingPageResponseFromDomain(page app.ListingPage) listingPageResponse {
	items := make([]listingResponse, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, listingResponseFromDomain(listing))
	}
	return listingPageResponse{Items: items, NextCursor: page.NextCursor}
}

func listingIDString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func listingIDFromPath(c *echo.Context) (int64, error) {
	value, err := strconv.ParseInt(c.Param("listing_id"), 10, 64)
	if err != nil || value < 1 {
		return 0, listingNotFoundError()
	}
	return value, nil
}

func listingNotFoundError() error {
	return &Error{Status: http.StatusNotFound, Code: "listing_not_found", Message: "Listing was not found."}
}

func catalogError(err error) error {
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
		return listingNotFoundError()
	case errors.Is(err, domain.ErrForbidden):
		return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "This operation is not allowed.", Err: err}
	case errors.Is(err, domain.ErrConflict):
		return &Error{Status: http.StatusConflict, Code: "invalid_listing_transition", Message: "The requested listing status transition is not allowed.", Err: err}
	default:
		return (&Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "An unexpected server error occurred."}).Wrap(err)
	}
}

func searchListingOptions(c *echo.Context) (app.SearchListingsOptions, error) {
	minimumPrice, err := optionalQueryInt64(c, "min_price")
	if err != nil {
		return app.SearchListingsOptions{}, err
	}
	maximumPrice, err := optionalQueryInt64(c, "max_price")
	if err != nil {
		return app.SearchListingsOptions{}, err
	}
	limit, err := optionalQueryInt(c, "limit")
	if err != nil {
		return app.SearchListingsOptions{}, err
	}
	var condition *domain.ListingCondition
	if value, present := c.QueryParams()["condition"]; present {
		parsed := domain.ListingCondition(value[0])
		condition = &parsed
	}
	return app.SearchListingsOptions{
		Query: c.QueryParam("q"), Category: c.QueryParam("category"), Condition: condition,
		MinimumPrice: minimumPrice, MaximumPrice: maximumPrice, Sort: app.ListingSort(c.QueryParam("sort")),
		Cursor: c.QueryParam("cursor"), Limit: limit,
	}, nil
}

func ownedListingOptions(c *echo.Context) (app.OwnedListingOptions, error) {
	limit, err := optionalQueryInt(c, "limit")
	if err != nil {
		return app.OwnedListingOptions{}, err
	}
	var status *domain.ListingStatus
	if value, present := c.QueryParams()["status"]; present {
		parsed := domain.ListingStatus(value[0])
		status = &parsed
	}
	return app.OwnedListingOptions{Status: status, Cursor: c.QueryParam("cursor"), Limit: limit}, nil
}

func optionalQueryInt64(c *echo.Context, name string) (*int64, error) {
	values, present := c.QueryParams()[name]
	if !present || len(values) == 0 {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		return nil, malformedQueryError(name, "must be an integer")
	}
	return &parsed, nil
}

func optionalQueryInt(c *echo.Context, name string) (int, error) {
	values, present := c.QueryParams()[name]
	if !present || len(values) == 0 {
		return 0, nil
	}
	parsed, err := strconv.Atoi(values[0])
	if err != nil {
		return 0, malformedQueryError(name, "must be an integer")
	}
	return parsed, nil
}

func malformedQueryError(field, message string) error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed.", Fields: map[string]string{field: message}}
}

type jsonField[T any] struct {
	present bool
	null    bool
	value   T
}

func (f *jsonField[T]) UnmarshalJSON(data []byte) error {
	f.present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.null = true
		return nil
	}
	return json.Unmarshal(data, &f.value)
}

type createListingRequest struct {
	Title        jsonField[string] `json:"title"`
	Description  jsonField[string] `json:"description"`
	PriceIDR     jsonField[int64]  `json:"price_idr"`
	Quantity     jsonField[int32]  `json:"quantity"`
	CategorySlug jsonField[string] `json:"category_slug"`
	Condition    jsonField[string] `json:"condition"`
	Negotiable   jsonField[bool]   `json:"negotiable"`
}

func (r createListingRequest) input() (app.CreateListingInput, error) {
	fields := requiredFieldErrors(map[string]jsonFieldMarker{
		"title": r.Title, "price_idr": r.PriceIDR, "category_slug": r.CategorySlug, "condition": r.Condition,
	})
	appendNullFieldErrors(fields, map[string]jsonFieldMarker{
		"title": r.Title, "description": r.Description, "price_idr": r.PriceIDR, "quantity": r.Quantity,
		"category_slug": r.CategorySlug, "condition": r.Condition, "negotiable": r.Negotiable,
	})
	if len(fields) > 0 {
		return app.CreateListingInput{}, domain.NewValidationError(fields)
	}
	input := app.CreateListingInput{Title: r.Title.value, PriceIDR: r.PriceIDR.value, CategorySlug: r.CategorySlug.value, Condition: domain.ListingCondition(r.Condition.value)}
	if r.Description.present {
		input.Description = r.Description.value
	}
	if r.Quantity.present {
		input.Quantity = r.Quantity.value
	} else {
		input.Quantity = 1
	}
	if r.Negotiable.present {
		input.Negotiable = r.Negotiable.value
	}
	return input, nil
}

type updateListingRequest struct {
	Title        jsonField[string] `json:"title"`
	Description  jsonField[string] `json:"description"`
	PriceIDR     jsonField[int64]  `json:"price_idr"`
	Quantity     jsonField[int32]  `json:"quantity"`
	CategorySlug jsonField[string] `json:"category_slug"`
	Condition    jsonField[string] `json:"condition"`
	Negotiable   jsonField[bool]   `json:"negotiable"`
}

func (r updateListingRequest) input() (app.UpdateListingInput, error) {
	fields := map[string]string{}
	all := map[string]jsonFieldMarker{
		"title": r.Title, "description": r.Description, "price_idr": r.PriceIDR, "quantity": r.Quantity,
		"category_slug": r.CategorySlug, "condition": r.Condition, "negotiable": r.Negotiable,
	}
	appendNullFieldErrors(fields, all)
	if !hasPresentField(all) {
		fields["body"] = "must include at least one listing field"
	}
	if len(fields) > 0 {
		return app.UpdateListingInput{}, domain.NewValidationError(fields)
	}
	input := app.UpdateListingInput{}
	if r.Title.present {
		input.Title = fieldPointer(r.Title.value)
	}
	if r.Description.present {
		input.Description = fieldPointer(r.Description.value)
	}
	if r.PriceIDR.present {
		input.PriceIDR = fieldPointer(r.PriceIDR.value)
	}
	if r.Quantity.present {
		input.Quantity = fieldPointer(r.Quantity.value)
	}
	if r.CategorySlug.present {
		input.CategorySlug = fieldPointer(r.CategorySlug.value)
	}
	if r.Condition.present {
		value := domain.ListingCondition(r.Condition.value)
		input.Condition = &value
	}
	if r.Negotiable.present {
		input.Negotiable = fieldPointer(r.Negotiable.value)
	}
	return input, nil
}

type changeListingStatusRequest struct {
	Status jsonField[string] `json:"status"`
}

func (r changeListingStatusRequest) status() (domain.ListingStatus, error) {
	fields := requiredFieldErrors(map[string]jsonFieldMarker{"status": r.Status})
	appendNullFieldErrors(fields, map[string]jsonFieldMarker{"status": r.Status})
	if len(fields) > 0 {
		return "", domain.NewValidationError(fields)
	}
	return domain.ListingStatus(r.Status.value), nil
}

type jsonFieldMarker interface {
	isPresent() bool
	isNull() bool
}

func (f jsonField[T]) isPresent() bool { return f.present }
func (f jsonField[T]) isNull() bool    { return f.null }

func requiredFieldErrors(fields map[string]jsonFieldMarker) map[string]string {
	result := map[string]string{}
	for name, field := range fields {
		if !field.isPresent() {
			result[name] = "is required"
		}
	}
	return result
}

func appendNullFieldErrors(result map[string]string, fields map[string]jsonFieldMarker) {
	for name, field := range fields {
		if field.isNull() {
			result[name] = "must not be null"
		}
	}
}

func hasPresentField(fields map[string]jsonFieldMarker) bool {
	for _, field := range fields {
		if field.isPresent() {
			return true
		}
	}
	return false
}

func fieldPointer[T any](value T) *T {
	return &value
}

func decodeListingRequest(c *echo.Context, destination any) error {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || !strings.EqualFold(mediaType, echo.MIMEApplicationJSON) {
		return echo.ErrUnsupportedMediaType
	}
	if c.Request().ContentLength > maximumListingRequestBytes {
		return echo.ErrStatusRequestEntityTooLarge
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maximumListingRequestBytes)
	decoder := json.NewDecoder(c.Request().Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return decodeListingRequestError(err)
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return malformedListingBodyError()
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return malformedListingBodyError()
		}
		return decodeListingRequestError(err)
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(destination); err != nil {
		return malformedListingBodyError()
	}
	return nil
}

func decodeListingRequestError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return echo.ErrStatusRequestEntityTooLarge
	}
	return malformedListingBodyError()
}

func malformedListingBodyError() error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed."}
}
