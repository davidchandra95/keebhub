package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

const maximumJSONRequestBytes = 32 << 10

// DecodeJSON decodes exactly one JSON value and rejects unknown fields.
func DecodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode JSON body: multiple JSON values")
		}
		return fmt.Errorf("decode JSON body trailer: %w", err)
	}
	return nil
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

// decodeJSONRequest enforces the shared JSON policy for authenticated JSON writes.
func decodeJSONRequest(c *echo.Context, destination any) error {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || !strings.EqualFold(mediaType, echo.MIMEApplicationJSON) {
		return echo.ErrUnsupportedMediaType
	}
	if c.Request().ContentLength > maximumJSONRequestBytes {
		return echo.ErrStatusRequestEntityTooLarge
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maximumJSONRequestBytes)
	decoder := json.NewDecoder(c.Request().Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return decodeJSONRequestError(err)
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return malformedJSONBodyError()
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return malformedJSONBodyError()
		}
		return decodeJSONRequestError(err)
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(destination); err != nil {
		return malformedJSONBodyError()
	}
	return nil
}

func decodeJSONRequestError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return echo.ErrStatusRequestEntityTooLarge
	}
	return malformedJSONBodyError()
}

func malformedJSONBodyError() error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "The request was malformed."}
}
