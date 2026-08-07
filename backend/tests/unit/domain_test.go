package unit

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the domain layer: the error taxonomy rendered as RFC 7807
// problem documents, and the operation name resolution used by the structured
// calculate payload.

// errorTypeBase is the documentation namespace every problem `type` URI is
// built from. It is part of the public API contract.
const errorTypeBase = "https://api.calculator.com/errors/"

func TestErrorConstructors_CarryCodeStatusAndTypeURI(t *testing.T) {
	cases := []struct {
		name     string
		err      *domain.AppError
		wantCode string
		wantHTTP int
		wantType string
	}{
		{"syntax", domain.NewSyntaxError("d"), domain.CodeSyntaxError, http.StatusBadRequest, "syntax-error"},
		{"invalid character", domain.NewInvalidCharacterError("d"), domain.CodeInvalidCharacter, http.StatusBadRequest, "invalid-character"},
		{"too long", domain.NewExpressionTooLongError("d"), domain.CodeExpressionTooLong, http.StatusBadRequest, "expression-too-long"},
		{"too deep", domain.NewNestingTooDeepError("d"), domain.CodeNestingTooDeep, http.StatusBadRequest, "nesting-too-deep"},
		{"division by zero", domain.NewDivisionByZeroError("d"), domain.CodeDivisionByZero, http.StatusBadRequest, "division-by-zero"},
		{"negative sqrt", domain.NewNegativeSqrtError("d"), domain.CodeNegativeSqrt, http.StatusBadRequest, "negative-square-root"},
		{"overflow", domain.NewNumericOverflowError("d"), domain.CodeNumericOverflow, http.StatusBadRequest, "numeric-overflow"},
		{"validation", domain.NewValidationError("d"), domain.CodeValidationError, http.StatusBadRequest, "validation-error"},
		{"malformed json", domain.NewMalformedJSONError("d"), domain.CodeMalformedJSON, http.StatusBadRequest, "malformed-json"},
		{"payload too large", domain.NewPayloadTooLargeError("d"), domain.CodePayloadTooLarge, http.StatusRequestEntityTooLarge, "payload-too-large"},
		{"unauthorized", domain.NewUnauthorizedError("d"), domain.CodeUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{"rate limit", domain.NewRateLimitError("d"), domain.CodeRateLimitExceeded, http.StatusTooManyRequests, "rate-limit-exceeded"},
		{"not found", domain.NewNotFoundError("d"), domain.CodeNotFound, http.StatusNotFound, "not-found"},
		{"method not allowed", domain.NewMethodNotAllowedError("d"), domain.CodeMethodNotAllowed, http.StatusMethodNotAllowed, "method-not-allowed"},
		{"internal", domain.NewInternalError("d"), domain.CodeInternal, http.StatusInternalServerError, "internal-error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantCode, tc.err.Code)
			assert.Equal(t, tc.wantHTTP, tc.err.Status)
			assert.Equal(t, errorTypeBase+tc.wantType, tc.err.Type)
			assert.NotEmpty(t, tc.err.Title)
			assert.Equal(t, "d", tc.err.Detail)
			assert.True(t, strings.HasPrefix(tc.err.Error(), tc.wantCode))
		})
	}
}

func TestAsAppError_UnwrapsWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("layer: %w", domain.NewSyntaxError("bad"))

	appErr, ok := domain.AsAppError(wrapped)

	require.True(t, ok)
	assert.Equal(t, domain.CodeSyntaxError, appErr.Code)
}

func TestAsAppError_IgnoresPlainErrors(t *testing.T) {
	_, ok := domain.AsAppError(errors.New("boom"))

	assert.False(t, ok)
}

func TestOperatorFor_ResolvesNamesAndAliases(t *testing.T) {
	cases := map[string]string{
		"add":      "+",
		"  ADD  ":  "+",
		"plus":     "+",
		"subtract": "-",
		"minus":    "-",
		"multiply": "*",
		"times":    "*",
		"divide":   "/",
		"division": "/",
		"power":    "^",
		"pow":      "^",
		"exponent": "^",
		"modulo":   "%",
		"mod":      "%",
	}

	for operation, want := range cases {
		t.Run(operation, func(t *testing.T) {
			symbol, ok := domain.OperatorFor(operation)
			assert.True(t, ok)
			assert.Equal(t, want, symbol)
		})
	}
}

func TestOperatorFor_RejectsUnknownOperations(t *testing.T) {
	// "percentage" is excluded on purpose: '%' is modulo, and answering a
	// percentage request with a remainder would be silently wrong.
	for _, operation := range []string{"", "factorial", "sqrt", "eval", "percentage"} {
		_, ok := domain.OperatorFor(operation)
		assert.False(t, ok, operation)
	}
}

func TestIsSquareRootOperation(t *testing.T) {
	for _, operation := range []string{"sqrt", "SQRT", " squareroot ", "square_root", "root"} {
		assert.True(t, domain.IsSquareRootOperation(operation), operation)
	}
	for _, operation := range []string{"add", "power", ""} {
		assert.False(t, domain.IsSquareRootOperation(operation), operation)
	}
}

func TestSupportedOperations_AreAllResolvable(t *testing.T) {
	for _, operation := range domain.SupportedOperations {
		if domain.IsSquareRootOperation(operation) {
			continue
		}
		_, ok := domain.OperatorFor(operation)
		assert.True(t, ok, operation)
	}
}
