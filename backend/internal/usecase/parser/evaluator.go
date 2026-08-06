package parser

import (
	"fmt"
	"math"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// precisionDigits is the number of decimal places kept when normalising a
// result. It hides IEEE-754 artifacts such as 0.1+0.2 = 0.30000000000000004
// without meaningfully reducing accuracy.
const precisionDigits = 10

// EvaluateRPN evaluates a Reverse Polish Notation token stream on an explicit
// value stack. Every intermediate result is checked for NaN/Inf so that
// overflow is reported as a domain error instead of leaking into the response.
func EvaluateRPN(rpn []Token) (float64, error) {
	stack := make([]float64, 0, len(rpn))

	for _, tok := range rpn {
		switch tok.Type {
		case TokenNumber:
			stack = append(stack, tok.Number)

		case TokenOperator:
			info, ok := operators[tok.Value]
			if !ok {
				return 0, domain.NewSyntaxError(fmt.Sprintf("Unknown operator %q.", tok.Value))
			}
			if info.unary {
				if len(stack) < 1 {
					return 0, domain.NewSyntaxError("Malformed expression: missing operand for a sign.")
				}
				operand := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				value, err := ApplyUnary(tok.Value, operand)
				if err != nil {
					return 0, err
				}
				stack = append(stack, value)
				continue
			}
			if len(stack) < 2 {
				return 0, domain.NewSyntaxError(fmt.Sprintf(
					"Malformed expression: operator %q is missing an operand.", tok.Value))
			}
			right := stack[len(stack)-1]
			left := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			value, err := ApplyBinary(tok.Value, left, right)
			if err != nil {
				return 0, err
			}
			stack = append(stack, value)

		case TokenFunction:
			// An unknown name has arity 0, which would let the guard below pass
			// on an empty stack and panic on the pop. Reject it first.
			arity, known := functions[tok.Value]
			if !known {
				return 0, domain.NewSyntaxError(fmt.Sprintf("Unsupported function %q.", tok.Value))
			}
			if len(stack) < arity {
				return 0, domain.NewSyntaxError(fmt.Sprintf(
					"Malformed expression: function %q is missing an argument.", tok.Value))
			}
			operand := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			value, err := ApplyFunction(tok.Value, operand)
			if err != nil {
				return 0, err
			}
			stack = append(stack, value)

		default:
			return 0, domain.NewSyntaxError("Malformed expression: unexpected parenthesis in evaluation stage.")
		}
	}

	if len(stack) != 1 {
		return 0, domain.NewSyntaxError("Malformed expression: operands and operators do not balance.")
	}
	return Round(stack[0]), nil
}

// ApplyBinary applies a binary operator, mapping every mathematically invalid
// case onto a typed domain error.
//
// `%` is the modulo operator (math.Mod). It shares the multiplicative
// precedence tier with `*` and `/`.
func ApplyBinary(op string, left, right float64) (float64, error) {
	var result float64

	switch op {
	case "+":
		result = left + right
	case "-":
		result = left - right
	case "*":
		result = left * right
	case "/":
		if right == 0 {
			return 0, domain.NewDivisionByZeroError(fmt.Sprintf(
				"Division by zero encountered in sub-expression '%s / %s'.",
				FormatNumber(left), FormatNumber(right)))
		}
		result = left / right
	case "%":
		if right == 0 {
			return 0, domain.NewDivisionByZeroError(fmt.Sprintf(
				"Modulo by zero encountered in sub-expression '%s %% %s'.",
				FormatNumber(left), FormatNumber(right)))
		}
		result = math.Mod(left, right)
	case "^":
		result = math.Pow(left, right)
		if math.IsNaN(result) {
			return 0, domain.NewNumericOverflowError(fmt.Sprintf(
				"Power operation '%s ^ %s' is undefined for real numbers.",
				FormatNumber(left), FormatNumber(right)))
		}
	default:
		return 0, domain.NewSyntaxError(fmt.Sprintf("Unsupported operator %q.", op))
	}

	return checkFinite(result, fmt.Sprintf("%s %s %s", FormatNumber(left), op, FormatNumber(right)))
}

// ApplyUnary applies a sign operator.
func ApplyUnary(op string, operand float64) (float64, error) {
	switch op {
	case unaryMinus:
		return -operand, nil
	case unaryPlus:
		return operand, nil
	default:
		return 0, domain.NewSyntaxError(fmt.Sprintf("Unsupported unary operator %q.", op))
	}
}

// ApplyFunction applies a whitelisted function.
func ApplyFunction(name string, operand float64) (float64, error) {
	switch name {
	case "sqrt":
		if operand < 0 {
			return 0, domain.NewNegativeSqrtError(fmt.Sprintf(
				"Square root of a negative number is undefined: 'sqrt(%s)'.", FormatNumber(operand)))
		}
		return checkFinite(math.Sqrt(operand), fmt.Sprintf("sqrt(%s)", FormatNumber(operand)))
	default:
		return 0, domain.NewSyntaxError(fmt.Sprintf("Unsupported function %q.", name))
	}
}

// EnsureFinite validates that an externally supplied value is a finite
// float64, returning a domain error otherwise.
func EnsureFinite(value float64) (float64, error) {
	return checkFinite(value, FormatNumber(value))
}

// checkFinite converts non-finite float64 results into a domain error.
func checkFinite(value float64, subExpression string) (float64, error) {
	switch {
	case math.IsNaN(value):
		return 0, domain.NewNumericOverflowError(fmt.Sprintf(
			"Sub-expression '%s' produced an undefined result (NaN).", subExpression))
	case math.IsInf(value, 0):
		return 0, domain.NewNumericOverflowError(fmt.Sprintf(
			"Sub-expression '%s' overflows the 64-bit floating point range.", subExpression))
	default:
		return value, nil
	}
}

// Round normalises a float64 to precisionDigits decimal places. Values large
// enough for the scaling factor to lose precision are returned unchanged.
func Round(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	if math.Abs(value) >= 1e15 {
		return value
	}
	factor := math.Pow(10, precisionDigits)
	rounded := math.Round(value*factor) / factor
	if rounded == 0 {
		// Avoid returning negative zero in JSON responses.
		return 0
	}
	return rounded
}
