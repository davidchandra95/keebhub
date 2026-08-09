package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const listingWriteBodyLimit int64 = 32 << 10

// Catalog is the application behavior consumed by the catalog HTTP handlers.
type Catalog interface {
	ListCategories(ctx context.Context) ([]domain.Category, error)
	CreateListing(ctx context.Context, seller domain.User, input app.CreateListingInput) (domain.Listing, error)
	UpdateListing(ctx context.Context, seller domain.User, listingID int64, input app.UpdateListingInput) (domain.Listing, error)
	ChangeListingStatus(ctx context.Context, seller domain.User, listingID int64, status domain.ListingStatus) (app.ListingStatusChange, error)
	ListOwnedListings(ctx context.Context, seller domain.User, input app.OwnedListingsInput) (app.ListingPage, error)
	GetListing(ctx context.Context, listingID int64, viewer *domain.User) (domain.Listing, error)
	SearchListings(ctx context.Context, input app.SearchListingsInput) (app.ListingPage, error)
}

type catalogHandlers struct {
	catalog Catalog
	logger  *zap.Logger
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
}

type listingResponse struct {
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	PriceIDR    int64                   `json:"price_idr"`
	Quantity    int                     `json:"quantity"`
	Category    categoryResponse        `json:"category"`
	Condition   domain.ListingCondition `json:"condition"`
	Status      domain.ListingStatus    `json:"status"`
	Negotiable  bool                    `json:"negotiable"`
	Seller      publicUserResponse      `json:"seller"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
}

type listingEnvelope struct {
	Listing listingResponse `json:"listing"`
}

type listingPageResponse struct {
	Items      []listingResponse `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

type optionalJSONField[T any] struct {
	Present bool
	Null    bool
	Value   T
}

func (f *optionalJSONField[T]) UnmarshalJSON(value []byte) error {
	f.Present = true
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		f.Null = true
		return nil
	}
	return json.Unmarshal(value, &f.Value)
}

type createListingRequest struct {
	Title        optionalJSONField[string] `json:"title"`
	Description  optionalJSONField[string] `json:"description"`
	PriceIDR     optionalJSONField[int64]  `json:"price_idr"`
	Quantity     optionalJSONField[int]    `json:"quantity"`
	CategorySlug optionalJSONField[string] `json:"category_slug"`
	Condition    optionalJSONField[string] `json:"condition"`
	Negotiable   optionalJSONField[bool]   `json:"negotiable"`
}

type updateListingRequest struct {
	Title        optionalJSONField[string] `json:"title"`
	Description  optionalJSONField[string] `json:"description"`
	PriceIDR     optionalJSONField[int64]  `json:"price_idr"`
	Quantity     optionalJSONField[int]    `json:"quantity"`
	CategorySlug optionalJSONField[string] `json:"category_slug"`
	Condition    optionalJSONField[string] `json:"condition"`
	Negotiable   optionalJSONField[bool]   `json:"negotiable"`
}

type changeListingStatusRequest struct {
	Status optionalJSONField[string] `json:"status"`
}

func newCatalogHandlers(cfg Config) catalogHandlers {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return catalogHandlers{catalog: cfg.Catalog, logger: logger}
}

func (h catalogHandlers) listCategories(c *echo.Context) error {
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	categories, err := h.catalog.ListCategories(c.Request().Context())
	if err != nil {
		return catalogHTTPError(err)
	}
	items := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryResponse{
			ID:   strconv.FormatInt(category.ID, 10),
			Slug: category.Slug,
			Name: category.Name,
		})
	}
	return c.JSON(http.StatusOK, categoriesResponse{Items: items})
}

func (h catalogHandlers) searchListings(c *echo.Context) error {
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	input, err := searchListingsInput(c)
	if err != nil {
		return catalogBadRequest(err)
	}
	page, err := h.catalog.SearchListings(c.Request().Context(), input)
	if err != nil {
		return catalogHTTPError(err)
	}
	return c.JSON(http.StatusOK, responseListingPage(page))
}

func (h catalogHandlers) createListing(c *echo.Context) error {
	seller, err := currentCatalogUser(c)
	if err != nil {
		return err
	}
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}

	var request createListingRequest
	if err := decodeListingJSON(c, &request); err != nil {
		return err
	}
	input, err := request.createInput()
	if err != nil {
		return err
	}
	listing, err := h.catalog.CreateListing(c.Request().Context(), seller, input)
	if err != nil {
		return catalogHTTPError(err)
	}
	h.logger.Info("Listing created",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", seller.ID),
		zap.Int64("listing_id", listing.ID),
	)
	return c.JSON(http.StatusCreated, listingEnvelope{Listing: responseListing(listing)})
}

func (h catalogHandlers) getListing(c *echo.Context) error {
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return listingNotFoundError()
	}

	var viewer *domain.User
	if user, ok := CurrentUser(c); ok {
		viewer = &user
	}
	listing, err := h.catalog.GetListing(c.Request().Context(), listingID, viewer)
	if err != nil {
		return catalogHTTPError(err)
	}
	return c.JSON(http.StatusOK, listingEnvelope{Listing: responseListing(listing)})
}

func (h catalogHandlers) updateListing(c *echo.Context) error {
	seller, err := currentCatalogUser(c)
	if err != nil {
		return err
	}
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return listingNotFoundError()
	}

	var request updateListingRequest
	if err := decodeListingJSON(c, &request); err != nil {
		return err
	}
	input, err := request.updateInput()
	if err != nil {
		return err
	}
	listing, err := h.catalog.UpdateListing(c.Request().Context(), seller, listingID, input)
	if err != nil {
		return catalogHTTPError(err)
	}
	h.logger.Info("Listing updated",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", seller.ID),
		zap.Int64("listing_id", listing.ID),
	)
	return c.JSON(http.StatusOK, listingEnvelope{Listing: responseListing(listing)})
}

func (h catalogHandlers) changeListingStatus(c *echo.Context) error {
	seller, err := currentCatalogUser(c)
	if err != nil {
		return err
	}
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return listingNotFoundError()
	}

	var request changeListingStatusRequest
	if err := decodeListingJSON(c, &request); err != nil {
		return err
	}
	status, err := request.statusInput()
	if err != nil {
		return err
	}
	change, err := h.catalog.ChangeListingStatus(c.Request().Context(), seller, listingID, status)
	if err != nil {
		return catalogHTTPError(err)
	}
	h.logger.Info("Listing status changed",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", seller.ID),
		zap.Int64("listing_id", change.Listing.ID),
		zap.String("old_status", string(change.OldStatus)),
		zap.String("new_status", string(change.Listing.Status)),
	)
	return c.JSON(http.StatusOK, listingEnvelope{Listing: responseListing(change.Listing)})
}

func (h catalogHandlers) listOwnedListings(c *echo.Context) error {
	seller, err := currentCatalogUser(c)
	if err != nil {
		return err
	}
	if h.catalog == nil {
		return internalCatalogError(errors.New("catalog service is not configured"))
	}
	input, err := ownedListingsInput(c)
	if err != nil {
		return catalogBadRequest(err)
	}
	page, err := h.catalog.ListOwnedListings(c.Request().Context(), seller, input)
	if err != nil {
		return catalogHTTPError(err)
	}
	return c.JSON(http.StatusOK, responseListingPage(page))
}

func (r createListingRequest) createInput() (app.CreateListingInput, error) {
	fields := make(map[string]string)
	validateRequiredField(fields, "title", r.Title)
	validateRequiredField(fields, "price_idr", r.PriceIDR)
	validateRequiredField(fields, "category_slug", r.CategorySlug)
	validateRequiredField(fields, "condition", r.Condition)
	validateNullableField(fields, "description", r.Description)
	validateNullableField(fields, "quantity", r.Quantity)
	validateNullableField(fields, "negotiable", r.Negotiable)
	if len(fields) > 0 {
		return app.CreateListingInput{}, listingValidationError(fields)
	}

	input := app.CreateListingInput{
		Title:        r.Title.Value,
		Description:  "",
		PriceIDR:     r.PriceIDR.Value,
		Quantity:     1,
		CategorySlug: r.CategorySlug.Value,
		Condition:    domain.ListingCondition(r.Condition.Value),
	}
	if r.Description.Present {
		input.Description = r.Description.Value
	}
	if r.Quantity.Present {
		input.Quantity = r.Quantity.Value
	}
	if r.Negotiable.Present {
		input.Negotiable = r.Negotiable.Value
	}
	return input, nil
}

func (r updateListingRequest) updateInput() (app.UpdateListingInput, error) {
	fields := make(map[string]string)
	validateNullableField(fields, "title", r.Title)
	validateNullableField(fields, "description", r.Description)
	validateNullableField(fields, "price_idr", r.PriceIDR)
	validateNullableField(fields, "quantity", r.Quantity)
	validateNullableField(fields, "category_slug", r.CategorySlug)
	validateNullableField(fields, "condition", r.Condition)
	validateNullableField(fields, "negotiable", r.Negotiable)
	if len(fields) > 0 {
		return app.UpdateListingInput{}, listingValidationError(fields)
	}
	if !r.Title.Present && !r.Description.Present && !r.PriceIDR.Present && !r.Quantity.Present &&
		!r.CategorySlug.Present && !r.Condition.Present && !r.Negotiable.Present {
		return app.UpdateListingInput{}, listingValidationError(map[string]string{
			"body": "must contain at least one listing field",
		})
	}

	input := app.UpdateListingInput{}
	if r.Title.Present {
		value := r.Title.Value
		input.Title = &value
	}
	if r.Description.Present {
		value := r.Description.Value
		input.Description = &value
	}
	if r.PriceIDR.Present {
		value := r.PriceIDR.Value
		input.PriceIDR = &value
	}
	if r.Quantity.Present {
		value := r.Quantity.Value
		input.Quantity = &value
	}
	if r.CategorySlug.Present {
		value := r.CategorySlug.Value
		input.CategorySlug = &value
	}
	if r.Condition.Present {
		value := domain.ListingCondition(r.Condition.Value)
		input.Condition = &value
	}
	if r.Negotiable.Present {
		value := r.Negotiable.Value
		input.Negotiable = &value
	}
	return input, nil
}

func (r changeListingStatusRequest) statusInput() (domain.ListingStatus, error) {
	fields := make(map[string]string)
	validateRequiredField(fields, "status", r.Status)
	if len(fields) > 0 {
		return "", listingValidationError(fields)
	}
	return domain.ListingStatus(r.Status.Value), nil
}

func validateRequiredField[T any](fields map[string]string, name string, field optionalJSONField[T]) {
	if !field.Present {
		fields[name] = "is required"
		return
	}
	if field.Null {
		fields[name] = "must not be null"
	}
}

func validateNullableField[T any](fields map[string]string, name string, field optionalJSONField[T]) {
	if field.Present && field.Null {
		fields[name] = "must not be null"
	}
}

func decodeListingJSON(c *echo.Context, destination any) error {
	if err := requireJSONContentType(c.Request()); err != nil {
		return err
	}
	request := c.Request()
	request.Body = http.MaxBytesReader(c.Response(), request.Body, listingWriteBodyLimit)
	if err := DecodeJSON(request, destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return (&Error{
				Status:  http.StatusRequestEntityTooLarge,
				Code:    "request_too_large",
				Message: "The request body is too large.",
			}).Wrap(err)
		}
		return (&Error{
			Status:  http.StatusBadRequest,
			Code:    "bad_request",
			Message: "The request was malformed.",
		}).Wrap(err)
	}
	return nil
}

func requireJSONContentType(request *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return &Error{
			Status:  http.StatusUnsupportedMediaType,
			Code:    "unsupported_media_type",
			Message: "The request content type is not supported.",
		}
	}
	return nil
}

func searchListingsInput(c *echo.Context) (app.SearchListingsInput, error) {
	query, err := singleQueryValue(c, "q")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	category, err := singleQueryValue(c, "category")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	condition, err := singleQueryValue(c, "condition")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	sort, err := singleQueryValue(c, "sort")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	cursor, err := singleQueryValue(c, "cursor")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	limit, err := queryPageLimit(c)
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	minPrice, err := optionalQueryInt64(c, "min_price")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	maxPrice, err := optionalQueryInt64(c, "max_price")
	if err != nil {
		return app.SearchListingsInput{}, err
	}
	return app.SearchListingsInput{
		Query:     query,
		Category:  category,
		Condition: condition,
		MinPrice:  minPrice,
		MaxPrice:  maxPrice,
		Sort:      sort,
		Cursor:    cursor,
		Limit:     limit,
	}, nil
}

func ownedListingsInput(c *echo.Context) (app.OwnedListingsInput, error) {
	status, err := singleQueryValue(c, "status")
	if err != nil {
		return app.OwnedListingsInput{}, err
	}
	cursor, err := singleQueryValue(c, "cursor")
	if err != nil {
		return app.OwnedListingsInput{}, err
	}
	limit, err := queryPageLimit(c)
	if err != nil {
		return app.OwnedListingsInput{}, err
	}
	return app.OwnedListingsInput{Status: status, Cursor: cursor, Limit: limit}, nil
}

func singleQueryValue(c *echo.Context, name string) (string, error) {
	values, present := c.QueryParams()[name]
	if !present {
		return "", nil
	}
	if len(values) != 1 {
		return "", errors.New("query parameter must not be repeated")
	}
	return values[0], nil
}

func queryPageLimit(c *echo.Context) (int, error) {
	value, present := c.QueryParams()["limit"]
	if !present {
		return 0, nil
	}
	if len(value) != 1 || value[0] == "" {
		return 0, errors.New("limit is malformed")
	}
	parsed, err := strconv.Atoi(value[0])
	if err != nil {
		return 0, errors.New("limit is malformed")
	}
	return parsed, nil
}

func optionalQueryInt64(c *echo.Context, name string) (*int64, error) {
	value, present := c.QueryParams()[name]
	if !present {
		return nil, nil
	}
	if len(value) != 1 || value[0] == "" {
		return nil, errors.New(name + " is malformed")
	}
	parsed, err := strconv.ParseInt(value[0], 10, 64)
	if err != nil {
		return nil, errors.New(name + " is malformed")
	}
	return &parsed, nil
}

func parseListingID(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("listing ID is empty")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, errors.New("listing ID is malformed")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("listing ID is malformed")
	}
	return parsed, nil
}

func currentCatalogUser(c *echo.Context) (domain.User, error) {
	user, ok := CurrentUser(c)
	if !ok {
		return domain.User{}, echo.ErrUnauthorized
	}
	return user, nil
}

func responseListing(listing domain.Listing) listingResponse {
	return listingResponse{
		ID:          strconv.FormatInt(listing.ID, 10),
		Title:       listing.Title,
		Description: listing.Description,
		PriceIDR:    listing.PriceIDR,
		Quantity:    listing.Quantity,
		Category: categoryResponse{
			ID:   strconv.FormatInt(listing.Category.ID, 10),
			Slug: listing.Category.Slug,
			Name: listing.Category.Name,
		},
		Condition:  listing.Condition,
		Status:     listing.Status,
		Negotiable: listing.Negotiable,
		Seller: publicUserResponse{
			ID:          strconv.FormatInt(listing.Seller.ID, 10),
			Handle:      listing.Seller.Handle,
			DisplayName: listing.Seller.DisplayName,
			AvatarURL:   listing.Seller.AvatarURL,
			Location:    listing.Seller.Location,
		},
		CreatedAt: listing.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: listing.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func responseListingPage(page app.ListingPage) listingPageResponse {
	items := make([]listingResponse, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, responseListing(listing))
	}
	return listingPageResponse{Items: items, NextCursor: page.NextCursor}
}

func listingValidationError(fields map[string]string) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "One or more fields are invalid.",
		Fields:  fields,
	}
}

func catalogBadRequest(err error) error {
	var queryError *app.CatalogQueryError
	fields := map[string]string(nil)
	if errors.As(err, &queryError) {
		fields = map[string]string{queryError.Field: queryError.Message}
	}
	return (&Error{
		Status:  http.StatusBadRequest,
		Code:    "bad_request",
		Message: "The request was malformed.",
		Fields:  fields,
	}).Wrap(err)
}

func catalogHTTPError(err error) error {
	var validationError *domain.ValidationError
	switch {
	case errors.As(err, &validationError):
		return listingValidationError(map[string]string{validationError.Field: validationError.Message}).Wrap(err)
	case errors.Is(err, app.ErrInvalidCatalogQuery):
		return catalogBadRequest(err)
	case errors.Is(err, domain.ErrListingNotFound), errors.Is(err, domain.ErrCategoryNotFound):
		return listingNotFoundError().Wrap(err)
	case errors.Is(err, domain.ErrForbidden):
		return (&Error{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "This operation is not allowed.",
		}).Wrap(err)
	case errors.Is(err, domain.ErrUserDisabled):
		return echo.ErrUnauthorized
	case errors.Is(err, domain.ErrConflict):
		return (&Error{
			Status:  http.StatusConflict,
			Code:    "conflict",
			Message: "The requested listing status change is not allowed.",
		}).Wrap(err)
	default:
		return internalCatalogError(err)
	}
}

func listingNotFoundError() *Error {
	return &Error{
		Status:  http.StatusNotFound,
		Code:    "listing_not_found",
		Message: "The requested listing was not found.",
	}
}

func internalCatalogError(err error) error {
	return (&Error{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "An unexpected server error occurred.",
	}).Wrap(err)
}
