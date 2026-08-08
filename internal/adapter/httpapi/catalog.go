package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const listingWriteBodyLimit int64 = 32 * 1024

type Catalog interface {
	ListCategories(context.Context) ([]domain.Category, error)
	CreateListing(context.Context, int64, app.CreateListingInput) (domain.Listing, error)
	UpdateListing(context.Context, int64, int64, app.UpdateListingInput) (domain.Listing, error)
	ChangeListingStatus(context.Context, int64, int64, domain.ListingStatus) (domain.Listing, error)
	ListOwnedListings(context.Context, int64, app.ListOwnedListingsInput) (app.ListingPage, error)
	GetListing(context.Context, int64, *int64) (domain.Listing, error)
	SearchListings(context.Context, app.SearchListingsInput) (app.ListingPage, error)
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

type publicUserResponse struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Location    *string `json:"location"`
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

func newCatalogHandlers(cfg Config) catalogHandlers {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return catalogHandlers{catalog: cfg.Catalog, logger: logger}
}

func (h catalogHandlers) listCategories(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	items, err := h.catalog.ListCategories(c.Request().Context())
	if err != nil {
		return err
	}
	response := make([]categoryResponse, 0, len(items))
	for _, item := range items {
		response = append(response, mapCategoryResponse(item))
	}
	return c.JSON(http.StatusOK, map[string]any{"items": response})
}

func (h catalogHandlers) searchListings(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	input, err := parseSearchListingsInput(c)
	if err != nil {
		return err
	}
	page, err := h.catalog.SearchListings(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, mapListingPageResponse(page))
}

func (h catalogHandlers) createListing(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	user, err := requireCurrentUser(c)
	if err != nil {
		return err
	}
	input, err := decodeCreateListingRequest(c)
	if err != nil {
		return err
	}
	listing, err := h.catalog.CreateListing(c.Request().Context(), user.ID, input)
	if err != nil {
		return err
	}
	h.logger.Info("Listing created",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", user.ID),
		zap.Int64("listing_id", listing.ID),
	)
	return c.JSON(http.StatusCreated, listingEnvelope{Listing: mapListingResponse(listing)})
}

func (h catalogHandlers) updateListing(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	user, err := requireCurrentUser(c)
	if err != nil {
		return err
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return err
	}
	input, err := decodeUpdateListingRequest(c)
	if err != nil {
		return err
	}
	listing, err := h.catalog.UpdateListing(c.Request().Context(), user.ID, listingID, input)
	if err != nil {
		return err
	}
	h.logger.Info("Listing updated",
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", user.ID),
		zap.Int64("listing_id", listing.ID),
	)
	return c.JSON(http.StatusOK, listingEnvelope{Listing: mapListingResponse(listing)})
}

func (h catalogHandlers) changeListingStatus(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	user, err := requireCurrentUser(c)
	if err != nil {
		return err
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return err
	}
	status, err := decodeStatusRequest(c)
	if err != nil {
		return err
	}
	oldStatus := ""
	if previous, getErr := h.catalog.GetListing(c.Request().Context(), listingID, &user.ID); getErr == nil {
		oldStatus = string(previous.Status)
	}
	listing, err := h.catalog.ChangeListingStatus(c.Request().Context(), user.ID, listingID, status)
	if err != nil {
		return err
	}
	fields := []zap.Field{
		zap.String("request_id", RequestID(c)),
		zap.Int64("seller_id", user.ID),
		zap.Int64("listing_id", listing.ID),
		zap.String("new_status", string(listing.Status)),
	}
	if oldStatus != "" {
		fields = append(fields, zap.String("old_status", oldStatus))
	}
	h.logger.Info("Listing status changed", fields...)
	return c.JSON(http.StatusOK, listingEnvelope{Listing: mapListingResponse(listing)})
}

func (h catalogHandlers) listOwnedListings(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	user, err := requireCurrentUser(c)
	if err != nil {
		return err
	}
	input, err := parseOwnedListingsInput(c)
	if err != nil {
		return err
	}
	page, err := h.catalog.ListOwnedListings(c.Request().Context(), user.ID, input)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, mapListingPageResponse(page))
}

func (h catalogHandlers) getListing(c *echo.Context) error {
	if h.catalog == nil {
		return errors.New("catalog service is not configured")
	}
	listingID, err := parseListingID(c.Param("listing_id"))
	if err != nil {
		return err
	}
	var viewerID *int64
	if user, ok := CurrentUser(c); ok {
		viewerID = &user.ID
	}
	listing, err := h.catalog.GetListing(c.Request().Context(), listingID, viewerID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, listingEnvelope{Listing: mapListingResponse(listing)})
}

func requireCurrentUser(c *echo.Context) (domain.User, error) {
	user, ok := CurrentUser(c)
	if !ok || user.Status == domain.UserStatusDisabled {
		return domain.User{}, echo.ErrUnauthorized
	}
	return user, nil
}

func parseListingID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.ErrNotFound
	}
	return id, nil
}

func parseOwnedListingsInput(c *echo.Context) (app.ListOwnedListingsInput, error) {
	query := c.Request().URL.Query()
	input := app.ListOwnedListingsInput{Cursor: query.Get("cursor")}
	if _, ok := query["status"]; ok {
		value := query.Get("status")
		if value == "" {
			return input, badQueryError("status", "Status is invalid.")
		}
		input.Status = domain.ListingStatus(value)
	}
	limit, err := parseLimitQuery(query)
	if err != nil {
		return input, err
	}
	input.Limit = limit
	return input, nil
}

func parseSearchListingsInput(c *echo.Context) (app.SearchListingsInput, error) {
	query := c.Request().URL.Query()
	input := app.SearchListingsInput{
		Query: query.Get("q"), Category: query.Get("category"), Cursor: query.Get("cursor"), Sort: app.SortNewest,
	}
	if _, ok := query["condition"]; ok {
		condition := query.Get("condition")
		if condition == "" {
			return input, badQueryError("condition", "Condition filter is invalid.")
		}
		valueCondition := domain.ListingCondition(condition)
		input.Condition = &valueCondition
	}
	var err error
	if input.MinPrice, err = parseOptionalPrice(query, "min_price"); err != nil {
		return input, err
	}
	if input.MaxPrice, err = parseOptionalPrice(query, "max_price"); err != nil {
		return input, err
	}
	if _, ok := query["sort"]; ok {
		if query.Get("sort") == "" {
			return input, badQueryError("sort", "Sort is invalid.")
		}
		input.Sort = app.ListingSort(query.Get("sort"))
	}
	if input.Sort == "" {
		input.Sort = app.SortNewest
	}
	input.Limit, err = parseLimitQuery(query)
	if err != nil {
		return input, err
	}
	return input, nil
}

func parseLimitQuery(query url.Values) (int, error) {
	if _, ok := query["limit"]; !ok {
		return 0, nil
	}
	value := query.Get("limit")
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, badQueryError("limit", "Limit is invalid.")
	}
	if limit < 1 || limit > app.MaximumListingPageLimit {
		return 0, badQueryError("limit", "Limit must be between 1 and 100.")
	}
	return limit, nil
}

func parseOptionalPrice(query url.Values, field string) (*int64, error) {
	if _, ok := query[field]; !ok {
		return nil, nil
	}
	value := query.Get(field)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, badQueryError(field, "Price is invalid.")
	}
	return &parsed, nil
}

func decodeCreateListingRequest(c *echo.Context) (app.CreateListingInput, error) {
	raw, err := decodeListingObject(c, map[string]bool{
		"title": true, "description": true, "price_idr": true, "quantity": true,
		"category_slug": true, "condition": true, "negotiable": true,
	})
	if err != nil {
		return app.CreateListingInput{}, err
	}
	fields := make(map[string]string)
	input := app.CreateListingInput{Quantity: 1}
	input.Title, _ = readRequiredString(raw, "title", fields)
	input.Description, _ = readOptionalString(raw, "description", "", fields)
	input.PriceIDR, _ = readRequiredInt64(raw, "price_idr", fields)
	input.Quantity, _ = readOptionalInt32(raw, "quantity", 1, fields)
	input.QuantitySet = hasRawField(raw, "quantity")
	input.CategorySlug, _ = readRequiredString(raw, "category_slug", fields)
	condition, _ := readRequiredString(raw, "condition", fields)
	input.Condition = domain.ListingCondition(condition)
	input.Negotiable, _ = readOptionalBool(raw, "negotiable", false, fields)
	if len(fields) > 0 {
		return app.CreateListingInput{}, listingJSONValidationError(fields)
	}
	return input, nil
}

func decodeUpdateListingRequest(c *echo.Context) (app.UpdateListingInput, error) {
	raw, err := decodeListingObject(c, map[string]bool{
		"title": true, "description": true, "price_idr": true, "quantity": true,
		"category_slug": true, "condition": true, "negotiable": true,
	})
	if err != nil {
		return app.UpdateListingInput{}, err
	}
	if len(raw) == 0 {
		return app.UpdateListingInput{}, listingJSONValidationError(map[string]string{"listing": "At least one field is required."})
	}
	fields := make(map[string]string)
	var input app.UpdateListingInput
	if _, ok := raw["title"]; ok {
		value, valid := readString(raw["title"], "title", fields)
		if valid {
			input.Title = &value
		}
	}
	if _, ok := raw["description"]; ok {
		value, valid := readString(raw["description"], "description", fields)
		if valid {
			input.Description = &value
		}
	}
	if _, ok := raw["price_idr"]; ok {
		value, valid := readInt64(raw["price_idr"], "price_idr", fields)
		if valid {
			input.PriceIDR = &value
		}
	}
	if _, ok := raw["quantity"]; ok {
		value, valid := readInt32(raw["quantity"], "quantity", fields)
		if valid {
			input.Quantity = &value
		}
	}
	if _, ok := raw["category_slug"]; ok {
		value, valid := readString(raw["category_slug"], "category_slug", fields)
		if valid {
			input.CategorySlug = &value
		}
	}
	if _, ok := raw["condition"]; ok {
		value, valid := readString(raw["condition"], "condition", fields)
		if valid {
			condition := domain.ListingCondition(value)
			input.Condition = &condition
		}
	}
	if _, ok := raw["negotiable"]; ok {
		value, valid := readBool(raw["negotiable"], "negotiable", fields)
		if valid {
			input.Negotiable = &value
		}
	}
	if len(fields) > 0 {
		return app.UpdateListingInput{}, listingJSONValidationError(fields)
	}
	return input, nil
}

func decodeStatusRequest(c *echo.Context) (domain.ListingStatus, error) {
	raw, err := decodeListingObject(c, map[string]bool{"status": true})
	if err != nil {
		return "", err
	}
	fields := make(map[string]string)
	value, _ := readRequiredString(raw, "status", fields)
	if len(fields) > 0 {
		return "", listingJSONValidationError(fields)
	}
	return domain.ListingStatus(value), nil
}

func decodeListingObject(c *echo.Context, allowed map[string]bool) (map[string]json.RawMessage, error) {
	contentType := c.Request().Header.Get(echo.HeaderContentType)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, echo.MIMEApplicationJSON) {
		return nil, &Error{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "The request content type is not supported."}
	}
	request := c.Request()
	if request.ContentLength > listingWriteBodyLimit {
		return nil, &Error{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "The request body is too large."}
	}
	if request.Body == nil {
		return nil, listingJSONValidationError(map[string]string{"body": "Request body is required."})
	}
	request.Body = http.MaxBytesReader(c.Response(), request.Body, listingWriteBodyLimit)
	decoder := json.NewDecoder(request.Body)
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		if isBodyTooLargeError(err) {
			return nil, &Error{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "The request body is too large."}
		}
		return nil, listingJSONValidationError(map[string]string{"body": "Request body must contain one JSON object."})
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if isBodyTooLargeError(err) {
			return nil, &Error{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "The request body is too large."}
		}
		return nil, listingJSONValidationError(map[string]string{"body": "Request body must contain one JSON value."})
	}
	if raw == nil {
		return nil, listingJSONValidationError(map[string]string{"body": "Request body must be a JSON object."})
	}
	for field := range raw {
		if !allowed[field] {
			return nil, listingJSONValidationError(map[string]string{field: "Unknown field."})
		}
	}
	return raw, nil
}

func isBodyTooLargeError(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}

func readRequiredString(raw map[string]json.RawMessage, field string, fields map[string]string) (string, bool) {
	value, ok := raw[field]
	if !ok {
		fields[field] = "This field is required."
		return "", false
	}
	return readString(value, field, fields)
}

func readOptionalString(raw map[string]json.RawMessage, field, fallback string, fields map[string]string) (string, bool) {
	value, ok := raw[field]
	if !ok {
		return fallback, true
	}
	return readString(value, field, fields)
}

func readString(raw json.RawMessage, field string, fields map[string]string) (string, bool) {
	if isJSONNull(raw) {
		fields[field] = "This field cannot be null."
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		fields[field] = "This field must be a string."
		return "", false
	}
	return value, true
}

func readRequiredInt64(raw map[string]json.RawMessage, field string, fields map[string]string) (int64, bool) {
	value, ok := raw[field]
	if !ok {
		fields[field] = "This field is required."
		return 0, false
	}
	return readInt64(value, field, fields)
}

func readOptionalInt32(raw map[string]json.RawMessage, field string, fallback int32, fields map[string]string) (int32, bool) {
	value, ok := raw[field]
	if !ok {
		return fallback, true
	}
	return readInt32(value, field, fields)
}

func readInt64(raw json.RawMessage, field string, fields map[string]string) (int64, bool) {
	if isJSONNull(raw) {
		fields[field] = "This field cannot be null."
		return 0, false
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		fields[field] = "This field must be an integer."
		return 0, false
	}
	return value, true
}

func readInt32(raw json.RawMessage, field string, fields map[string]string) (int32, bool) {
	if isJSONNull(raw) {
		fields[field] = "This field cannot be null."
		return 0, false
	}
	var value int32
	if err := json.Unmarshal(raw, &value); err != nil {
		fields[field] = "This field must be an integer."
		return 0, false
	}
	return value, true
}

func readOptionalBool(raw map[string]json.RawMessage, field string, fallback bool, fields map[string]string) (bool, bool) {
	value, ok := raw[field]
	if !ok {
		return fallback, true
	}
	return readBool(value, field, fields)
}

func readBool(raw json.RawMessage, field string, fields map[string]string) (bool, bool) {
	if isJSONNull(raw) {
		fields[field] = "This field cannot be null."
		return false, false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		fields[field] = "This field must be a boolean."
		return false, false
	}
	return value, true
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.EqualFold(strings.TrimSpace(string(raw)), "null")
}

func hasRawField(raw map[string]json.RawMessage, field string) bool {
	_, ok := raw[field]
	return ok
}

func listingJSONValidationError(fields map[string]string) error {
	return &Error{Status: http.StatusBadRequest, Code: "validation_failed", Message: "Request validation failed.", Fields: fields}
}

func badQueryError(field, message string) error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed.", Fields: map[string]string{field: message}}
}

func mapCategoryResponse(category domain.Category) categoryResponse {
	return categoryResponse{ID: strconv.FormatInt(category.ID, 10), Slug: category.Slug, Name: category.Name}
}

func mapListingResponse(listing domain.Listing) listingResponse {
	return listingResponse{
		ID:    strconv.FormatInt(listing.ID, 10),
		Title: listing.Title, Description: listing.Description, PriceIDR: listing.PriceIDR,
		Quantity: listing.Quantity, Category: mapCategoryResponse(listing.Category),
		Condition: string(listing.Condition), Status: string(listing.Status), Negotiable: listing.Negotiable,
		Seller: publicUserResponse{
			ID: strconv.FormatInt(listing.Seller.ID, 10), Handle: listing.Seller.Handle,
			DisplayName: listing.Seller.DisplayName, AvatarURL: listing.Seller.AvatarURL, Location: listing.Seller.Location,
		},
		CreatedAt: formatTimestamp(listing.CreatedAt), UpdatedAt: formatTimestamp(listing.UpdatedAt),
	}
}

func mapListingPageResponse(page app.ListingPage) listingPageResponse {
	items := make([]listingResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, mapListingResponse(item))
	}
	return listingPageResponse{Items: items, NextCursor: page.NextCursor}
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}
