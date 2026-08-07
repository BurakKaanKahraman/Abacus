// Package domain holds the core models, transport-agnostic DTOs and the
// application error taxonomy. It has no dependencies on any other internal
// package, which keeps the dependency graph pointing inwards (Clean Architecture).
package domain

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes returned to API clients. They are part of the public API
// contract, so they must stay stable.
const (
	CodeSyntaxError       = "ERR_SYNTAX_ERROR"
	CodeInvalidCharacter  = "ERR_INVALID_CHARACTER"
	CodeExpressionTooLong = "ERR_EXPRESSION_TOO_LONG"
	CodeNestingTooDeep    = "ERR_NESTING_TOO_DEEP"
	CodeDivisionByZero    = "ERR_DIVISION_BY_ZERO"
	CodeNegativeSqrt      = "ERR_NEGATIVE_SQRT"
	CodeNumericOverflow   = "ERR_NUMERIC_OVERFLOW"
	CodeValidationError   = "ERR_VALIDATION_ERROR"
	CodeMalformedJSON     = "ERR_MALFORMED_JSON"
	CodeUnauthorized      = "ERR_UNAUTHORIZED"
	CodeRateLimitExceeded = "ERR_RATE_LIMIT_EXCEEDED"
	CodeNotFound          = "ERR_NOT_FOUND"
	CodeMethodNotAllowed  = "ERR_METHOD_NOT_ALLOWED"
	CodePayloadTooLarge   = "ERR_PAYLOAD_TOO_LARGE"
	CodeInternal          = "ERR_INTERNAL_ERROR"
)

// errorTypeBase is the documentation namespace used for RFC 7807 `type` URIs.
const errorTypeBase = "https://api.calculator.com/errors/"

// AppError is the single error type crossing layer boundaries. Handlers render
// it directly as an RFC 7807 Problem Details document.
type AppError struct {
	Code   string // machine readable, e.g. ERR_SYNTAX_ERROR
	Title  string // short, human readable summary
	Detail string // specific, contextual explanation
	Status int    // HTTP status code
	Type   string // RFC 7807 type URI
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

// AsAppError extracts an *AppError from an error chain, reporting whether one
// was found.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

func newAppError(code, slug, title, detail string, status int) *AppError {
	return &AppError{
		Code:   code,
		Title:  title,
		Detail: detail,
		Status: status,
		Type:   errorTypeBase + slug,
	}
}

// NewSyntaxError reports a malformed expression (unbalanced parentheses,
// dangling operators, unexpected tokens).
func NewSyntaxError(detail string) *AppError {
	return newAppError(CodeSyntaxError, "syntax-error", "Invalid Expression Syntax", detail, http.StatusBadRequest)
}

// NewInvalidCharacterError reports a character or identifier that is not part
// of the calculator grammar (a likely injection attempt).
func NewInvalidCharacterError(detail string) *AppError {
	return newAppError(CodeInvalidCharacter, "invalid-character", "Invalid Expression Character", detail, http.StatusBadRequest)
}

// NewExpressionTooLongError reports an expression exceeding the configured
// character budget.
func NewExpressionTooLongError(detail string) *AppError {
	return newAppError(CodeExpressionTooLong, "expression-too-long", "Expression Too Long", detail, http.StatusBadRequest)
}

// NewNestingTooDeepError reports parentheses nested beyond the configured
// depth limit.
func NewNestingTooDeepError(detail string) *AppError {
	return newAppError(CodeNestingTooDeep, "nesting-too-deep", "Expression Nesting Too Deep", detail, http.StatusBadRequest)
}

// NewDivisionByZeroError reports a division or modulo by zero at any step of
// the evaluation chain.
func NewDivisionByZeroError(detail string) *AppError {
	return newAppError(CodeDivisionByZero, "division-by-zero", "Invalid Mathematical Operation", detail, http.StatusBadRequest)
}

// NewNegativeSqrtError reports a square root of a negative operand.
func NewNegativeSqrtError(detail string) *AppError {
	return newAppError(CodeNegativeSqrt, "negative-square-root", "Invalid Mathematical Operation", detail, http.StatusBadRequest)
}

// NewNumericOverflowError reports a result that is not a finite float64.
func NewNumericOverflowError(detail string) *AppError {
	return newAppError(CodeNumericOverflow, "numeric-overflow", "Numeric Overflow", detail, http.StatusBadRequest)
}

// NewValidationError reports a structurally valid but semantically invalid
// request payload.
func NewValidationError(detail string) *AppError {
	return newAppError(CodeValidationError, "validation-error", "Invalid Request Payload", detail, http.StatusBadRequest)
}

// NewMalformedJSONError reports a body that could not be decoded as JSON.
func NewMalformedJSONError(detail string) *AppError {
	return newAppError(CodeMalformedJSON, "malformed-json", "Malformed JSON Payload", detail, http.StatusBadRequest)
}

// NewPayloadTooLargeError reports a request body exceeding the size budget.
func NewPayloadTooLargeError(detail string) *AppError {
	return newAppError(CodePayloadTooLarge, "payload-too-large", "Payload Too Large", detail, http.StatusRequestEntityTooLarge)
}

// NewUnauthorizedError reports a missing, malformed or expired bearer token.
func NewUnauthorizedError(detail string) *AppError {
	return newAppError(CodeUnauthorized, "unauthorized", "Unauthorized", detail, http.StatusUnauthorized)
}

// NewRateLimitError reports a throttled client.
func NewRateLimitError(detail string) *AppError {
	return newAppError(CodeRateLimitExceeded, "rate-limit-exceeded", "Too Many Requests", detail, http.StatusTooManyRequests)
}

// NewNotFoundError reports an unknown route.
func NewNotFoundError(detail string) *AppError {
	return newAppError(CodeNotFound, "not-found", "Resource Not Found", detail, http.StatusNotFound)
}

// NewMethodNotAllowedError reports an unsupported HTTP verb on a known route.
func NewMethodNotAllowedError(detail string) *AppError {
	return newAppError(CodeMethodNotAllowed, "method-not-allowed", "Method Not Allowed", detail, http.StatusMethodNotAllowed)
}

// NewInternalError reports an unexpected server-side failure. The detail is
// intentionally generic so that internals are never leaked to clients.
func NewInternalError(detail string) *AppError {
	return newAppError(CodeInternal, "internal-error", "Internal Server Error", detail, http.StatusInternalServerError)
}
