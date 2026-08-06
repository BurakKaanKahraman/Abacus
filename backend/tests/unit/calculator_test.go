package unit

import (
	"testing"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the calculator application service: payload dispatch,
// N-operand folding and error propagation from the expression engine.

func newCalculator() *usecase.Calculator {
	return usecase.NewCalculator(parser.NewEngine(parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth))
}

func TestCalculator_ExpressionPayload(t *testing.T) {
	result, err := newCalculator().Calculate(domain.CalculateRequest{
		Expression: "10 + 20 * 3 - 15 / (5 - 2)",
	})

	require.NoError(t, err)
	assert.Equal(t, 65.0, result.Result)
	assert.Equal(t, "10 + 20 × 3 - 15 ÷ (5 - 2)", result.Expression)
	assert.Equal(t, "10 + 20 × 3 - 15 ÷ (5 - 2) = 65", result.Formatted)
}

func TestCalculator_OperandsPayload(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		operands  []float64
		expected  float64
		formatted string
	}{
		{"add", "add", []float64{15.5, 24.5, 10, 50}, 100, "15.5 + 24.5 + 10 + 50 = 100"},
		{"subtract", "subtract", []float64{100, 20, 30}, 50, "100 - 20 - 30 = 50"},
		{"multiply", "multiply", []float64{2, 3, 4}, 24, "2 × 3 × 4 = 24"},
		{"divide", "divide", []float64{100, 5, 2}, 10, "100 ÷ 5 ÷ 2 = 10"},
		{"power folds left to right", "power", []float64{2, 3, 2}, 64, "2 ^ 3 ^ 2 = 64"},
		{"modulo", "modulo", []float64{100, 7}, 2, "100 % 7 = 2"},
		{"sqrt", "sqrt", []float64{16}, 4, "√(16) = 4"},
		{"negative operands are parenthesised", "add", []float64{-5, 10}, 5, "(-5) + 10 = 5"},
		{"alias is case insensitive", "PLUS", []float64{1, 2}, 3, "1 + 2 = 3"},
		{"percentage aliases modulo", "percentage", []float64{10, 3}, 1, "10 % 3 = 1"},
		{"two operands minimum", "add", []float64{1, 2}, 3, "1 + 2 = 3"},
	}

	calc := newCalculator()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := calc.Calculate(domain.CalculateRequest{
				Operation: tc.operation,
				Operands:  tc.operands,
			})
			require.NoError(t, err)
			assert.InDelta(t, tc.expected, result.Result, 1e-9)
			assert.Equal(t, tc.formatted, result.Formatted)
		})
	}
}

func TestCalculator_OperandsPayload_SupportsArbitraryLength(t *testing.T) {
	operands := make([]float64, 0, usecase.MaxOperands)
	for i := 0; i < usecase.MaxOperands; i++ {
		operands = append(operands, 1)
	}

	result, err := newCalculator().Calculate(domain.CalculateRequest{
		Operation: domain.OpAdd,
		Operands:  operands,
	})

	require.NoError(t, err)
	assert.Equal(t, float64(usecase.MaxOperands), result.Result)
}

func TestCalculator_OperandsPayload_Validation(t *testing.T) {
	cases := []struct {
		name    string
		request domain.CalculateRequest
		code    string
		detail  string
	}{
		{
			name:    "missing operation",
			request: domain.CalculateRequest{Operands: []float64{1, 2}},
			code:    domain.CodeValidationError,
			detail:  "'operation' is required",
		},
		{
			name:    "unsupported operation",
			request: domain.CalculateRequest{Operation: "factorial", Operands: []float64{1, 2}},
			code:    domain.CodeValidationError,
			detail:  "Unsupported operation",
		},
		{
			name:    "empty operands",
			request: domain.CalculateRequest{Operation: domain.OpAdd},
			code:    domain.CodeValidationError,
			detail:  "at least one number",
		},
		{
			name:    "single operand for binary operation",
			request: domain.CalculateRequest{Operation: domain.OpAdd, Operands: []float64{1}},
			code:    domain.CodeValidationError,
			detail:  "at least 2 operands",
		},
		{
			name:    "too many operands",
			request: domain.CalculateRequest{Operation: domain.OpAdd, Operands: make([]float64, usecase.MaxOperands+1)},
			code:    domain.CodeValidationError,
			detail:  "at most 1000",
		},
		{
			name:    "sqrt with multiple operands",
			request: domain.CalculateRequest{Operation: domain.OpSquareRoot, Operands: []float64{1, 2}},
			code:    domain.CodeValidationError,
			detail:  "exactly 1 operand",
		},
		{
			name:    "division by zero",
			request: domain.CalculateRequest{Operation: domain.OpDivide, Operands: []float64{10, 0}},
			code:    domain.CodeDivisionByZero,
			detail:  "Division by zero",
		},
		{
			name:    "negative square root",
			request: domain.CalculateRequest{Operation: domain.OpSquareRoot, Operands: []float64{-9}},
			code:    domain.CodeNegativeSqrt,
			detail:  "negative",
		},
		{
			name:    "overflow",
			request: domain.CalculateRequest{Operation: domain.OpPower, Operands: []float64{9999, 9999}},
			code:    domain.CodeNumericOverflow,
			detail:  "overflow",
		},
	}

	calc := newCalculator()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := calc.Calculate(tc.request)
			appErr := requireAppError(t, err, tc.code)
			assert.Contains(t, appErr.Detail, tc.detail)
		})
	}
}

func TestCalculator_RejectsAmbiguousAndEmptyPayloads(t *testing.T) {
	calc := newCalculator()

	_, err := calc.Calculate(domain.CalculateRequest{
		Expression: "1 + 1",
		Operation:  domain.OpAdd,
		Operands:   []float64{1, 2},
	})
	appErr := requireAppError(t, err, domain.CodeValidationError)
	assert.Contains(t, appErr.Detail, "not both")

	_, err = calc.Calculate(domain.CalculateRequest{})
	appErr = requireAppError(t, err, domain.CodeValidationError)
	assert.Contains(t, appErr.Detail, "must contain either")
}

func TestCalculator_PropagatesExpressionErrors(t *testing.T) {
	calc := newCalculator()

	cases := []struct {
		expression string
		code       string
	}{
		{"10 + (20 * 3", domain.CodeSyntaxError},
		{"10 ++ 20", domain.CodeSyntaxError},
		{"eval('x')", domain.CodeInvalidCharacter},
		{"10 / (5 - 5)", domain.CodeDivisionByZero},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			_, err := calc.Calculate(domain.CalculateRequest{Expression: tc.expression})
			requireAppError(t, err, tc.code)
		})
	}
}

func TestNewCalculator_UsesDefaultEngineWhenNil(t *testing.T) {
	result, err := usecase.NewCalculator(nil).Calculate(domain.CalculateRequest{Expression: "2 + 2"})

	require.NoError(t, err)
	assert.Equal(t, 4.0, result.Result)
}
