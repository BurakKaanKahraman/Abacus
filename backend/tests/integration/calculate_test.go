package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculate_ExpressionPayload(t *testing.T) {
	router := newTestRouter(t, testConfig())

	cases := []struct {
		name       string
		expression string
		result     float64
		formatted  string
	}{
		{
			name:       "complex expression with precedence",
			expression: "10 + 20 * 3 - 15 / (5 - 2)",
			result:     65,
			formatted:  "10 + 20 × 3 - 15 ÷ (5 - 2) = 65",
		},
		{
			name:       "square root and power",
			expression: "(10 + sqrt(16)) * 2^3",
			result:     112,
			formatted:  "(10 + √(16)) × 2 ^ 3 = 112",
		},
		{
			name:       "unary minus with function",
			expression: "-10 + sqrt(16) * 2",
			result:     -2,
			formatted:  "-10 + √(16) × 2 = -2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := do(t, router, postJSON(t, "/api/v1/calculate",
				domain.CalculateRequest{Expression: tc.expression}))

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))

			var response domain.CalculateResponse
			decodeBody(t, recorder, &response)
			assert.Equal(t, tc.result, response.Result)
			assert.Equal(t, tc.formatted, response.Formatted)
			assert.NotEmpty(t, response.Expression)
			assert.NotEmpty(t, response.Timestamp)
		})
	}
}

func TestCalculate_OperandsPayload(t *testing.T) {
	router := newTestRouter(t, testConfig())

	recorder := do(t, router, postJSON(t, "/api/v1/calculate", domain.CalculateRequest{
		Operation: domain.OpAdd,
		Operands:  []float64{15.5, 24.5, 10, 50},
	}))

	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.CalculateResponse
	decodeBody(t, recorder, &response)
	assert.Equal(t, 100.0, response.Result)
	assert.Equal(t, "15.5 + 24.5 + 10 + 50 = 100", response.Formatted)
}

// TestCalculate_ArithmeticErrors verifies that each domain error code reaches
// the client as the documented problem document.
func TestCalculate_ArithmeticErrors(t *testing.T) {
	router := newTestRouter(t, testConfig())

	cases := []struct {
		name    string
		request domain.CalculateRequest
		status  int
		code    string
		detail  string
	}{
		{
			name:    "division by zero in sub-expression",
			request: domain.CalculateRequest{Expression: "10 + 20 * 3 - 15 / (5 - 5)"},
			status:  http.StatusBadRequest,
			code:    domain.CodeDivisionByZero,
			detail:  "Division by zero",
		},
		{
			name:    "unbalanced parentheses",
			request: domain.CalculateRequest{Expression: "10 + (20 * 3"},
			status:  http.StatusBadRequest,
			code:    domain.CodeSyntaxError,
			detail:  "Missing 1 closing",
		},
		{
			name:    "double operator",
			request: domain.CalculateRequest{Expression: "10 ++ 20"},
			status:  http.StatusBadRequest,
			code:    domain.CodeSyntaxError,
			detail:  "Double operator",
		},
		{
			name:    "code injection attempt",
			request: domain.CalculateRequest{Expression: "eval('1+1')"},
			status:  http.StatusBadRequest,
			code:    domain.CodeInvalidCharacter,
			detail:  "Unsupported identifier",
		},
		{
			name:    "negative square root",
			request: domain.CalculateRequest{Expression: "sqrt(0 - 16)"},
			status:  http.StatusBadRequest,
			code:    domain.CodeNegativeSqrt,
			detail:  "negative",
		},
		{
			name:    "numeric overflow",
			request: domain.CalculateRequest{Expression: "9999 ^ 9999"},
			status:  http.StatusBadRequest,
			code:    domain.CodeNumericOverflow,
			detail:  "overflow",
		},
		{
			name:    "expression too long",
			request: domain.CalculateRequest{Expression: strings.Repeat("1+", 300) + "1"},
			status:  http.StatusBadRequest,
			code:    domain.CodeExpressionTooLong,
			detail:  "500",
		},
		{
			name:    "nesting too deep",
			request: domain.CalculateRequest{Expression: strings.Repeat("(", 21) + "1" + strings.Repeat(")", 21)},
			status:  http.StatusBadRequest,
			code:    domain.CodeNestingTooDeep,
			detail:  "20",
		},
		{
			name:    "unsupported operation",
			request: domain.CalculateRequest{Operation: "factorial", Operands: []float64{1, 2}},
			status:  http.StatusBadRequest,
			code:    domain.CodeValidationError,
			detail:  "Unsupported operation",
		},
		{
			name:    "empty payload",
			request: domain.CalculateRequest{},
			status:  http.StatusBadRequest,
			code:    domain.CodeValidationError,
			detail:  "must contain either",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := do(t, router, postJSON(t, "/api/v1/calculate", tc.request))

			require.Equal(t, tc.status, recorder.Code)
			problem := decodeProblem(t, recorder)
			assert.Equal(t, tc.code, problem.Code)
			assert.Contains(t, problem.Detail, tc.detail)
			assert.Equal(t, "/api/v1/calculate", problem.Instance)
		})
	}
}

// TestCalculate_MalformedRequests covers the transport layer's own failure
// modes, which never reach the usecase.
func TestCalculate_MalformedRequests(t *testing.T) {
	router := newTestRouter(t, testConfig())

	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"broken json", `{"expression": "1+1"`, http.StatusBadRequest, domain.CodeMalformedJSON},
		{"empty body", ``, http.StatusBadRequest, domain.CodeMalformedJSON},
		{"not an object", `"just a string"`, http.StatusBadRequest, domain.CodeValidationError},
		{"wrong field type", `{"expression": 42}`, http.StatusBadRequest, domain.CodeValidationError},
		{"wrong operand type", `{"operation":"add","operands":["a","b"]}`, http.StatusBadRequest, domain.CodeValidationError},
		{"unknown field", `{"expresion": "1+1"}`, http.StatusBadRequest, domain.CodeValidationError},
		{"trailing document", `{"expression":"1+1"}{"expression":"2+2"}`, http.StatusBadRequest, domain.CodeMalformedJSON},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := do(t, router, postJSON(t, "/api/v1/calculate", tc.body))

			require.Equal(t, tc.status, recorder.Code)
			assert.Equal(t, tc.code, decodeProblem(t, recorder).Code)
		})
	}
}

func TestCalculate_RejectsNonJSONContentType(t *testing.T) {
	router := newTestRouter(t, testConfig())

	req := postJSON(t, "/api/v1/calculate", `{"expression":"1+1"}`)
	req.Header.Set("Content-Type", "text/plain")

	recorder := do(t, router, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, domain.CodeValidationError, decodeProblem(t, recorder).Code)
}

func TestCalculate_RejectsOversizedBody(t *testing.T) {
	cfg := testConfig()
	cfg.MaxRequestBodyBytes = 256
	router := newTestRouter(t, cfg)

	oversized := `{"expression":"` + strings.Repeat("1", 1024) + `"}`

	recorder := do(t, router, postJSON(t, "/api/v1/calculate", oversized))

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.Equal(t, domain.CodePayloadTooLarge, decodeProblem(t, recorder).Code)
}

// TestCalculate_LargeOperandArrayIsAccepted pins the "no parameter length
// limit" requirement: an array far beyond any keypad input still evaluates.
func TestCalculate_LargeOperandArrayIsAccepted(t *testing.T) {
	router := newTestRouter(t, testConfig())

	operands := make([]float64, 1000)
	for i := range operands {
		operands[i] = 1
	}

	recorder := do(t, router, postJSON(t, "/api/v1/calculate", domain.CalculateRequest{
		Operation: domain.OpAdd,
		Operands:  operands,
	}))

	require.Equal(t, http.StatusOK, recorder.Code)

	var response domain.CalculateResponse
	decodeBody(t, recorder, &response)
	assert.Equal(t, 1000.0, response.Result)
}
