package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// decodeJSON decodes a request body into dst, translating every decoder
// failure into a typed domain error. Unknown fields are rejected so that a
// typo in a field name surfaces as an error instead of being silently ignored.
func decodeJSON(r *http.Request, dst any) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		if mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0]); mediaType != "application/json" {
			return domain.NewValidationError(fmt.Sprintf(
				"Unsupported Content-Type %q. Expected application/json.", mediaType))
		}
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return decodeError(err)
	}

	// A well formed request carries exactly one JSON document.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.NewMalformedJSONError("Request body must contain a single JSON object.")
	}
	return nil
}

// decodeError maps encoding/json failures onto the domain error taxonomy.
func decodeError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxBytesErr *http.MaxBytesError

	switch {
	case errors.As(err, &maxBytesErr):
		return domain.NewPayloadTooLargeError(fmt.Sprintf(
			"Request body exceeds the maximum of %d bytes.", maxBytesErr.Limit))

	case errors.As(err, &syntaxErr):
		return domain.NewMalformedJSONError(fmt.Sprintf(
			"Request body contains malformed JSON at byte offset %d.", syntaxErr.Offset))

	case errors.As(err, &typeErr):
		return domain.NewValidationError(fmt.Sprintf(
			"Field %q expects a %s value.", typeErr.Field, typeErr.Type))

	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return domain.NewMalformedJSONError("Request body must not be empty.")

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return domain.NewValidationError(fmt.Sprintf("Unknown field %s in request body.", field))

	default:
		return domain.NewMalformedJSONError("Request body could not be decoded as JSON.")
	}
}
