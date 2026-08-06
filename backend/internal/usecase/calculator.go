// Package usecase contains the application services. It depends on domain and
// on the expression engine only, never on HTTP or any other delivery mechanism.
package usecase

import (
	"fmt"
	"strings"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase/parser"
)

// MaxOperands caps the structured payload to keep a single request bounded.
// The expression endpoint has no operand count limit beyond the character
// budget; this guard only prevents unbounded array payloads.
const MaxOperands = 1000

// Calculator is the application service behind POST /api/v1/calculate. It
// accepts both request shapes and routes them through the same arithmetic core.
type Calculator struct {
	engine *parser.Engine
}

// NewCalculator wires the calculator to an expression engine.
func NewCalculator(engine *parser.Engine) *Calculator {
	if engine == nil {
		engine = parser.NewEngine(parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
	}
	return &Calculator{engine: engine}
}

// Calculate dispatches on the request shape: a raw expression string goes
// through the Shunting-Yard engine, a structured payload is folded over the
// same operator primitives.
func (c *Calculator) Calculate(req domain.CalculateRequest) (domain.CalculationResult, error) {
	hasExpression := strings.TrimSpace(req.Expression) != ""
	hasOperands := len(req.Operands) > 0 || strings.TrimSpace(req.Operation) != ""

	switch {
	case hasExpression && hasOperands:
		return domain.CalculationResult{}, domain.NewValidationError(
			"Provide either an 'expression' string or an 'operation' with 'operands', not both.")
	case hasExpression:
		return c.evaluateExpression(req.Expression)
	case hasOperands:
		return c.evaluateOperands(req.Operation, req.Operands)
	default:
		return domain.CalculationResult{}, domain.NewValidationError(
			"Request must contain either an 'expression' string or an 'operation' with an 'operands' array.")
	}
}

// evaluateExpression runs the raw string through the full parser pipeline.
func (c *Calculator) evaluateExpression(expression string) (domain.CalculationResult, error) {
	evaluation, err := c.engine.Evaluate(expression)
	if err != nil {
		return domain.CalculationResult{}, err
	}
	return newResult(evaluation.Normalized, evaluation.Result), nil
}

// evaluateOperands folds an arbitrary length operand array with the requested
// operation, reusing the engine's operator primitives so that arithmetic
// semantics and error taxonomy stay identical across both endpoints shapes.
//
// Binary operations are folded left to right, matching the left-associative
// reading of the equivalent flat expression.
func (c *Calculator) evaluateOperands(operation string, operands []float64) (domain.CalculationResult, error) {
	if strings.TrimSpace(operation) == "" {
		return domain.CalculationResult{}, domain.NewValidationError(
			fmt.Sprintf("Field 'operation' is required. Supported operations: %s.",
				strings.Join(domain.SupportedOperations, ", ")))
	}
	if len(operands) == 0 {
		return domain.CalculationResult{}, domain.NewValidationError(
			"Field 'operands' must contain at least one number.")
	}
	if len(operands) > MaxOperands {
		return domain.CalculationResult{}, domain.NewValidationError(
			fmt.Sprintf("Field 'operands' accepts at most %d values, got %d.", MaxOperands, len(operands)))
	}
	for i, operand := range operands {
		if _, err := parser.EnsureFinite(operand); err != nil {
			return domain.CalculationResult{}, domain.NewValidationError(
				fmt.Sprintf("Operand at index %d is not a finite number.", i))
		}
	}

	if domain.IsSquareRootOperation(operation) {
		return c.squareRoot(operands)
	}

	symbol, ok := domain.OperatorFor(operation)
	if !ok {
		return domain.CalculationResult{}, domain.NewValidationError(
			fmt.Sprintf("Unsupported operation %q. Supported operations: %s.",
				operation, strings.Join(domain.SupportedOperations, ", ")))
	}
	if len(operands) < 2 {
		return domain.CalculationResult{}, domain.NewValidationError(
			fmt.Sprintf("Operation %q requires at least 2 operands, got %d.", operation, len(operands)))
	}

	accumulator := operands[0]
	var err error
	for _, operand := range operands[1:] {
		accumulator, err = parser.ApplyBinary(symbol, accumulator, operand)
		if err != nil {
			return domain.CalculationResult{}, err
		}
	}

	return newResult(joinOperands(operands, displaySymbol(symbol)), parser.Round(accumulator)), nil
}

// squareRoot handles the unary sqrt operation, which takes exactly one operand.
func (c *Calculator) squareRoot(operands []float64) (domain.CalculationResult, error) {
	if len(operands) != 1 {
		return domain.CalculationResult{}, domain.NewValidationError(
			fmt.Sprintf("Operation %q requires exactly 1 operand, got %d.", domain.OpSquareRoot, len(operands)))
	}
	value, err := parser.ApplyFunction("sqrt", operands[0])
	if err != nil {
		return domain.CalculationResult{}, err
	}
	expression := fmt.Sprintf("√(%s)", parser.FormatNumber(operands[0]))
	return newResult(expression, parser.Round(value)), nil
}

// newResult assembles the usecase result including the `expr = value` rendering.
func newResult(expression string, value float64) domain.CalculationResult {
	return domain.CalculationResult{
		Expression: expression,
		Result:     value,
		Formatted:  fmt.Sprintf("%s = %s", expression, parser.FormatNumber(value)),
	}
}

// joinOperands renders an operand array as an infix expression, parenthesising
// negative values so the output stays unambiguous.
func joinOperands(operands []float64, symbol string) string {
	parts := make([]string, 0, len(operands))
	for _, operand := range operands {
		formatted := parser.FormatNumber(operand)
		if operand < 0 {
			formatted = "(" + formatted + ")"
		}
		parts = append(parts, formatted)
	}
	return strings.Join(parts, " "+symbol+" ")
}

// displaySymbol maps an operator lexeme onto its typographic form.
func displaySymbol(symbol string) string {
	switch symbol {
	case "*":
		return "×"
	case "/":
		return "÷"
	default:
		return symbol
	}
}
