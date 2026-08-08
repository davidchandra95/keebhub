package httpapi

import (
	"errors"
	"net/http"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// Error is a safe HTTP error with an internal wrapped cause.
type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
	Err     error
}

// Error implements error without exposing the safe public message as internal context.
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

// Unwrap exposes the internal cause to errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.Err
}

// StatusCode lets Echo resolve response status for middleware logging.
func (e *Error) StatusCode() int {
	return e.Status
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Fields    map[string]string `json:"fields,omitempty"`
}

func errorHandler(logger *zap.Logger) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		if response, _ := echo.UnwrapResponse(c.Response()); response != nil && response.Committed {
			return
		}

		status, code, message, fields := classifyError(err)
		requestID := RequestID(c)
		if requestID == "" {
			requestID = "unavailable"
		}

		if status >= http.StatusInternalServerError {
			logger.Error("request failed",
				zap.String("request_id", requestID),
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.Error(err),
			)
		}

		if writeErr := c.JSON(status, errorEnvelope{Error: errorBody{
			Code:      code,
			Message:   message,
			RequestID: requestID,
			Fields:    fields,
		}}); writeErr != nil {
			logger.Error("write error response",
				zap.String("request_id", requestID),
				zap.Error(writeErr),
			)
		}
	}
}

func classifyError(err error) (int, string, string, map[string]string) {
	var apiError *Error
	if errors.As(err, &apiError) {
		return apiError.Status, apiError.Code, apiError.Message, apiError.Fields
	}
	var badRequest *app.BadRequestError
	if errors.As(err, &badRequest) {
		code := badRequest.Code
		if code == "" {
			code = "bad_request"
		}
		message := badRequest.Message
		if message == "" {
			message = "The request was malformed."
		}
		return http.StatusBadRequest, code, message, badRequest.Fields
	}
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		return http.StatusUnprocessableEntity, "validation_failed", "Request validation failed.", validation.Fields
	}
	if errors.Is(err, domain.ErrNotFound) {
		return http.StatusNotFound, "not_found", "The requested resource was not found.", nil
	}
	if errors.Is(err, domain.ErrForbidden) {
		return http.StatusForbidden, "forbidden", "This operation is not allowed.", nil
	}
	if errors.Is(err, domain.ErrConflict) {
		return http.StatusConflict, "conflict", "The requested state change is not allowed.", nil
	}

	status := echo.StatusCode(err)
	switch status {
	case http.StatusBadRequest:
		return status, "bad_request", "The request was malformed.", nil
	case http.StatusUnauthorized:
		return status, "authentication_required", "Authentication is required.", nil
	case http.StatusForbidden:
		return status, "forbidden", "This operation is not allowed.", nil
	case http.StatusNotFound:
		return status, "not_found", "The requested resource was not found.", nil
	case http.StatusMethodNotAllowed:
		return status, "method_not_allowed", "The request method is not allowed.", nil
	case http.StatusRequestEntityTooLarge:
		return status, "request_too_large", "The request body is too large.", nil
	case http.StatusUnsupportedMediaType:
		return status, "unsupported_media_type", "The request content type is not supported.", nil
	case http.StatusTooManyRequests:
		return status, "rate_limited", "Too many requests.", nil
	case http.StatusBadGateway:
		return status, "upstream_failure", "An external service request failed.", nil
	case http.StatusServiceUnavailable:
		return status, "service_unavailable", "The service is temporarily unavailable.", nil
	default:
		return http.StatusInternalServerError, "internal_error", "An unexpected server error occurred.", nil
	}
}
