package parser

import (
	"math"
	"strconv"
	"strings"
)

// displaySymbols maps internal lexemes onto the typographic symbols used in the
// human readable `formatted` response field.
var displaySymbols = map[string]string{
	"*":        "×",
	"/":        "÷",
	unaryMinus: "-",
	unaryPlus:  "+",
	"sqrt":     "√",
}

// FormatNumber renders a float64 without trailing zeros, falling back to
// scientific notation for very large or very small magnitudes.
func FormatNumber(value float64) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "Infinity"
	}
	if math.IsInf(value, -1) {
		return "-Infinity"
	}
	abs := math.Abs(value)
	if value != 0 && (abs >= 1e15 || abs < 1e-6) {
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// FormatExpression renders a token stream as a normalised, human readable
// expression (`10 + 20 × 3 - 15 ÷ (5 - 2)`). Spacing is derived from token
// types so that the output stays stable regardless of the user's input style.
func FormatExpression(tokens []Token) string {
	var b strings.Builder

	for i, tok := range tokens {
		if i > 0 && needsSpaceBefore(tokens[i-1], tok) {
			b.WriteByte(' ')
		}
		b.WriteString(symbolFor(tok))
	}
	return b.String()
}

func symbolFor(t Token) string {
	if symbol, ok := displaySymbols[t.Value]; ok && t.Type != TokenNumber {
		return symbol
	}
	if t.Type == TokenNumber {
		return FormatNumber(t.Number)
	}
	return t.Value
}

// needsSpaceBefore decides whether a separating space is inserted between two
// adjacent tokens. Unary signs, function names and parentheses hug their
// operand; binary operators are surrounded by spaces.
func needsSpaceBefore(prev, cur Token) bool {
	switch {
	case prev.Type == TokenFunction:
		return false
	case prev.Type == TokenLeftParen:
		return false
	case prev.IsUnary():
		return false
	case cur.Type == TokenRightParen:
		return false
	case cur.IsUnary():
		return true
	default:
		return true
	}
}
