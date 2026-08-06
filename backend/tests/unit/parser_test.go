// Package unit holds the black-box unit test suite for the backend. Tests live
// outside the production packages and exercise them through their public API
// only, which keeps the implementation free to change as long as the contract
// holds.
//
// This file covers the expression engine end to end and stage by stage:
//
//	sanitize -> tokenize -> shunting-yard (RPN) -> evaluate -> format
package unit

import (
	"math"
	"strings"
	"testing"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// Shared fixtures
// -----------------------------------------------------------------------------

// Internal lexemes for unary operators, mirrored here so hand written token
// streams stay readable.
const (
	unaryMinus = "u-"
	unaryPlus  = "u+"
)

// newEngine builds an engine with the production default limits.
func newEngine() *parser.Engine {
	return parser.NewEngine(parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
}

// mustTokenize tokenizes an expression, failing the test on error.
func mustTokenize(t *testing.T, expression string) []parser.Token {
	t.Helper()
	tokens, err := parser.Tokenize(expression)
	require.NoError(t, err)
	return tokens
}

// tokenValues extracts the lexemes of a token stream for concise assertions.
func tokenValues(tokens []parser.Token) []string {
	values := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		values = append(values, tok.Value)
	}
	return values
}

// toRPNString converts an infix expression to a space separated RPN string.
func toRPNString(t *testing.T, expression string) string {
	t.Helper()
	rpn, err := parser.ToRPN(mustTokenize(t, expression))
	require.NoError(t, err)
	return strings.Join(tokenValues(rpn), " ")
}

// num builds a number token for hand written RPN streams.
func num(value float64) parser.Token {
	return parser.Token{Type: parser.TokenNumber, Number: value}
}

// op builds an operator token for hand written RPN streams.
func op(value string) parser.Token {
	return parser.Token{Type: parser.TokenOperator, Value: value}
}

// fn builds a function token for hand written RPN streams.
func fn(name string) parser.Token {
	return parser.Token{Type: parser.TokenFunction, Value: name}
}

// requireAppError asserts that err is an *domain.AppError carrying the
// expected code, and returns it for further assertions.
func requireAppError(t *testing.T, err error, code string) *domain.AppError {
	t.Helper()
	require.Error(t, err)
	appErr, ok := domain.AsAppError(err)
	require.True(t, ok, "expected a domain.AppError, got %T", err)
	assert.Equal(t, code, appErr.Code)
	return appErr
}

// -----------------------------------------------------------------------------
// Stage 1 - Sanitizer (security boundary)
// -----------------------------------------------------------------------------

func TestSanitize_AcceptsValidExpressions(t *testing.T) {
	cases := []string{
		"1+1",
		"10 + 20 * 3 - 15 / (5 - 2)",
		"(10 + sqrt(16)) * 2^3",
		"-10 + sqrt(16) * 2",
		"  3.14 * 2  ",
		"100 % 7",
		"SQRT(9)",
	}

	for _, expression := range cases {
		t.Run(expression, func(t *testing.T) {
			sanitized, err := parser.Sanitize(expression, parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
			require.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(expression), sanitized)
		})
	}
}

func TestSanitize_RejectsInjectionAttempts(t *testing.T) {
	cases := []struct {
		name       string
		expression string
	}{
		{"javascript eval", "eval('1+1')"},
		{"shell command", "1 + $(rm -rf /)"},
		{"sql injection", "1; DROP TABLE users"},
		{"script tag", "<script>alert(1)</script>"},
		{"template injection", "{{7*7}}"},
		{"go identifier", "os.Exit(1)"},
		{"backticks", "`whoami`"},
		{"pipe operator", "1 | 2"},
		{"null byte", "1+1\x00"},
		{"unicode operator", "1 ＋ 1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Sanitize(tc.expression, parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
			require.Error(t, err)
			appErr, ok := domain.AsAppError(err)
			require.True(t, ok)
			assert.Contains(t, []string{domain.CodeInvalidCharacter, domain.CodeSyntaxError}, appErr.Code)
			assert.Equal(t, 400, appErr.Status)
		})
	}
}

func TestSanitize_RejectsEmptyExpression(t *testing.T) {
	for _, expression := range []string{"", "   ", "\t\n"} {
		_, err := parser.Sanitize(expression, 0, 0)
		requireAppError(t, err, domain.CodeValidationError)
	}
}

func TestSanitize_EnforcesMaxLength(t *testing.T) {
	expression := strings.Repeat("1+", 300) + "1" // 601 characters

	_, err := parser.Sanitize(expression, parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)

	appErr := requireAppError(t, err, domain.CodeExpressionTooLong)
	assert.Contains(t, appErr.Detail, "500")
}

func TestSanitize_EnforcesMaxNestingDepth(t *testing.T) {
	tooDeep := strings.Repeat("(", 21) + "1" + strings.Repeat(")", 21)
	_, err := parser.Sanitize(tooDeep, parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
	requireAppError(t, err, domain.CodeNestingTooDeep)

	atLimit := strings.Repeat("(", 20) + "1" + strings.Repeat(")", 20)
	_, err = parser.Sanitize(atLimit, parser.DefaultMaxExpressionLength, parser.DefaultMaxNestingDepth)
	require.NoError(t, err)
}

func TestSanitize_DetectsUnbalancedParentheses(t *testing.T) {
	cases := []struct {
		expression string
		detail     string
	}{
		{"10 + (20 * 3", "Missing 1 closing"},
		{"10 + 20) * 3", "Unexpected ')'"},
		{"((1+2)", "Missing 1 closing"},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			_, err := parser.Sanitize(tc.expression, 0, 0)
			appErr := requireAppError(t, err, domain.CodeSyntaxError)
			assert.Contains(t, appErr.Detail, tc.detail)
		})
	}
}

// -----------------------------------------------------------------------------
// Stage 2 - Tokenizer
// -----------------------------------------------------------------------------

func TestTokenize_ProducesExpectedTokenStream(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		expected   []string
	}{
		{"simple addition", "1+2", []string{"1", "+", "2"}},
		{"mixed operators", "10 + 20 * 3", []string{"10", "+", "20", "*", "3"}},
		{"decimals", "3.5 / 0.5", []string{"3.5", "/", "0.5"}},
		{"parentheses", "(1+2)*3", []string{"(", "1", "+", "2", ")", "*", "3"}},
		{"function call", "sqrt(16)", []string{"sqrt", "(", "16", ")"}},
		{"uppercase function", "SQRT(16)", []string{"sqrt", "(", "16", ")"}},
		{"leading unary minus", "-10 + 5", []string{unaryMinus, "10", "+", "5"}},
		{"leading unary plus", "+10", []string{unaryPlus, "10"}},
		{"unary after operator", "10 * -5", []string{"10", "*", unaryMinus, "5"}},
		{"unary after left paren", "(-5)", []string{"(", unaryMinus, "5", ")"}},
		{"unary in exponent", "2 ^ -3", []string{"2", "^", unaryMinus, "3"}},
		{"negation after subtraction", "5 - -3", []string{"5", "-", unaryMinus, "3"}},
		{"negation after addition", "5 + -3", []string{"5", "+", unaryMinus, "3"}},
		{"modulo", "10 % 3", []string{"10", "%", "3"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tokenValues(mustTokenize(t, tc.expression)))
		})
	}
}

func TestTokenize_RecordsPositionsAndNumbers(t *testing.T) {
	tokens := mustTokenize(t, "12 + 3.5")
	require.Len(t, tokens, 3)

	assert.Equal(t, parser.TokenNumber, tokens[0].Type)
	assert.InDelta(t, 12.0, tokens[0].Number, 1e-9)
	assert.Equal(t, 1, tokens[0].Pos)

	assert.Equal(t, parser.TokenOperator, tokens[1].Type)
	assert.Equal(t, 4, tokens[1].Pos)

	assert.InDelta(t, 3.5, tokens[2].Number, 1e-9)
	assert.Equal(t, 6, tokens[2].Pos)
}

func TestTokenize_RejectsSyntaxErrors(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		detail     string
	}{
		{"double plus", "10 ++ 20", "Double operator"},
		{"plus after minus", "10 -+ 20", "Double operator"},
		{"plus after multiplication", "10 * +5", "Double operator"},
		{"triple sign", "10 * --5", "Double operator"},
		{"stacked signs after subtraction", "10 - --5", "Double operator"},
		{"trailing operator", "10 +", "ends unexpectedly"},
		{"leading binary operator", "* 10", "an operand was expected"},
		{"two numbers", "10 20", "an operator was expected"},
		{"operator before closing paren", "(10 +)", "an operand was expected"},
		{"empty parentheses", "10 + ()", "Empty parentheses"},
		{"implicit multiplication", "2(3)", "an operator was expected"},
		{"number after closing paren", "(2)3", "an operator was expected"},
		{"function without parenthesis", "sqrt 16", "must be followed by '('"},
		{"malformed decimal", "1.2.3 + 1", "Malformed number"},
		{"lone dot", ". + 1", "Malformed number"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Tokenize(tc.expression)
			require.Error(t, err)
			appErr, ok := domain.AsAppError(err)
			require.True(t, ok)
			assert.Contains(t, []string{domain.CodeSyntaxError, domain.CodeInvalidCharacter}, appErr.Code)
			assert.Contains(t, appErr.Detail, tc.detail)
		})
	}
}

// TestTokenize_ReportsOutOfRangeLiteralAsOverflow separates a syntactically
// valid literal that float64 cannot hold from an actually malformed one.
func TestTokenize_ReportsOutOfRangeLiteralAsOverflow(t *testing.T) {
	huge := strings.Repeat("9", 400) // legal under the 500 character cap

	_, err := parser.Tokenize(huge)

	appErr := requireAppError(t, err, domain.CodeNumericOverflow)
	assert.Contains(t, appErr.Detail, "outside the representable float64 range")
	assert.Less(t, len(appErr.Detail), 200, "the offending literal must be truncated in the message")
}

// TestSanitize_RejectsNonASCIIWhitespace pins the whitelist and the tokenizer
// to the same notion of whitespace: an invisible character must never be
// silently skipped, since it could change how an expression parses.
func TestSanitize_RejectsNonASCIIWhitespace(t *testing.T) {
	cases := map[string]string{
		"non-breaking space": "1 + 2",
		"line separator":     "1 +\u20282",
		"ideographic space":  "1 +\u30002",
		"zero width space":   "1 +\u200B2",
	}

	for name, expression := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parser.Sanitize(expression, 0, 0)
			appErr := requireAppError(t, err, domain.CodeInvalidCharacter)
			assert.Contains(t, appErr.Detail, "at position", "the offending rune must be located")
		})
	}
}

func TestTokenize_RejectsUnknownIdentifier(t *testing.T) {
	_, err := parser.Tokenize("log(10)")

	appErr := requireAppError(t, err, domain.CodeInvalidCharacter)
	assert.Contains(t, appErr.Detail, "log")
}

func TestTokenize_RejectsEmptyInput(t *testing.T) {
	_, err := parser.Tokenize("   ")

	requireAppError(t, err, domain.CodeValidationError)
}

func TestToken_IsUnary(t *testing.T) {
	assert.True(t, op(unaryMinus).IsUnary())
	assert.True(t, op(unaryPlus).IsUnary())
	assert.False(t, op("-").IsUnary())
	assert.False(t, num(1).IsUnary())
}

func TestIsFunction(t *testing.T) {
	assert.True(t, parser.IsFunction("sqrt"))
	assert.False(t, parser.IsFunction("SQRT"), "lookups are performed on the lower-cased name")
	assert.False(t, parser.IsFunction("log"))
	assert.False(t, parser.IsFunction(""))
}

// -----------------------------------------------------------------------------
// Stage 3 - Shunting-Yard (precedence contract)
// -----------------------------------------------------------------------------

func TestToRPN_AppliesOperatorPrecedence(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		expected   string
	}{
		{"multiplication before addition", "10 + 20 * 3", "10 20 3 * +"},
		{"division before subtraction", "10 - 20 / 5", "10 20 5 / -"},
		{"left to right additive", "1 + 2 - 3", "1 2 + 3 -"},
		{"left to right multiplicative", "8 / 4 * 2", "8 4 / 2 *"},
		{"modulo shares multiplicative tier", "1 + 10 % 3", "1 10 3 % +"},
		{"power binds tightest", "2 + 3 ^ 2", "2 3 2 ^ +"},
		{"power is right associative", "2 ^ 3 ^ 2", "2 3 2 ^ ^"},
		{"parentheses override precedence", "(10 + 20) * 3", "10 20 + 3 *"},
		{"nested parentheses", "10 + 20 * 3 - 15 / (5 - 2)", "10 20 3 * + 15 5 2 - / -"},
		{"function call", "sqrt(16) + 1", "16 sqrt 1 +"},
		{"function inside expression", "(10 + sqrt(16)) * 2 ^ 3", "10 16 sqrt + 2 3 ^ *"},
		{"unary minus", "-10 + 5", "10 u- 5 +"},
		{"unary minus binds looser than power", "-2 ^ 2", "2 2 ^ u-"},
		{"unary minus binds tighter than multiplication", "-2 * 3", "2 u- 3 *"},
		{"unary minus on right operand", "10 * -5", "10 5 u- *"},
		{"unary minus in exponent", "2 ^ -3", "2 3 u- ^"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, toRPNString(t, tc.expression))
		})
	}
}

func TestToRPN_RejectsUnbalancedParenthesesDefensively(t *testing.T) {
	// The sanitizer normally catches this first; ToRPN must not panic when
	// handed an unbalanced stream directly.
	unclosed := []parser.Token{
		{Type: parser.TokenLeftParen, Value: "(", Pos: 1},
		num(1),
	}
	_, err := parser.ToRPN(unclosed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ')'")

	unopened := []parser.Token{
		num(1),
		{Type: parser.TokenRightParen, Value: ")", Pos: 2},
	}
	_, err = parser.ToRPN(unopened)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected ')'")
}

func TestToRPN_RejectsUnknownOperator(t *testing.T) {
	_, err := parser.ToRPN([]parser.Token{num(1), op("&"), num(2)})

	appErr := requireAppError(t, err, domain.CodeSyntaxError)
	assert.Contains(t, appErr.Detail, "Unknown operator")
}

// -----------------------------------------------------------------------------
// Stage 4 - Evaluator primitives
// -----------------------------------------------------------------------------

func TestApplyBinary_AppliesEachOperator(t *testing.T) {
	cases := []struct {
		name     string
		op       string
		left     float64
		right    float64
		expected float64
	}{
		{"add", "+", 15.5, 24.5, 40},
		{"subtract", "-", 10, 4, 6},
		{"multiply", "*", 6, 7, 42},
		{"divide", "/", 84, 2, 42},
		{"divide into fraction", "/", 1, 8, 0.125},
		{"modulo", "%", 10, 3, 1},
		{"modulo keeps dividend sign", "%", -10, 3, -1},
		{"power", "^", 2, 10, 1024},
		{"fractional power", "^", 9, 0.5, 3},
		{"negative exponent", "^", 2, -2, 0.25},
		{"zero exponent", "^", 12345, 0, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.ApplyBinary(tc.op, tc.left, tc.right)
			require.NoError(t, err)
			assert.InDelta(t, tc.expected, result, 1e-9)
		})
	}
}

func TestApplyBinary_DivisionAndModuloByZero(t *testing.T) {
	cases := []struct {
		name   string
		op     string
		detail string
	}{
		{"division", "/", "Division by zero encountered in sub-expression '10 / 0'."},
		{"modulo", "%", "Modulo by zero encountered in sub-expression '10 % 0'."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.ApplyBinary(tc.op, 10, 0)
			appErr := requireAppError(t, err, domain.CodeDivisionByZero)
			assert.Equal(t, 400, appErr.Status)
			assert.Equal(t, tc.detail, appErr.Detail)
		})
	}
}

func TestApplyBinary_RejectsNonFiniteResults(t *testing.T) {
	cases := []struct {
		name  string
		op    string
		left  float64
		right float64
	}{
		{"power overflow", "^", 9999, 9999},
		{"multiplication overflow", "*", math.MaxFloat64, 10},
		{"addition overflow", "+", math.MaxFloat64, math.MaxFloat64},
		{"undefined power of negative base", "^", -8, 0.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.ApplyBinary(tc.op, tc.left, tc.right)
			requireAppError(t, err, domain.CodeNumericOverflow)
		})
	}
}

func TestApplyBinary_RejectsUnknownOperator(t *testing.T) {
	_, err := parser.ApplyBinary("&", 1, 2)

	requireAppError(t, err, domain.CodeSyntaxError)
}

func TestApplyUnary(t *testing.T) {
	negated, err := parser.ApplyUnary(unaryMinus, 10)
	require.NoError(t, err)
	assert.Equal(t, -10.0, negated)

	kept, err := parser.ApplyUnary(unaryPlus, -10)
	require.NoError(t, err)
	assert.Equal(t, -10.0, kept)

	_, err = parser.ApplyUnary("u?", 1)
	requireAppError(t, err, domain.CodeSyntaxError)
}

func TestApplyFunction_SquareRoot(t *testing.T) {
	result, err := parser.ApplyFunction("sqrt", 144)
	require.NoError(t, err)
	assert.Equal(t, 12.0, result)

	result, err = parser.ApplyFunction("sqrt", 0)
	require.NoError(t, err)
	assert.Equal(t, 0.0, result)
}

func TestApplyFunction_RejectsNegativeOperandAndUnknownName(t *testing.T) {
	_, err := parser.ApplyFunction("sqrt", -16)
	appErr := requireAppError(t, err, domain.CodeNegativeSqrt)
	assert.Contains(t, appErr.Detail, "sqrt(-16)")

	_, err = parser.ApplyFunction("log", 1)
	requireAppError(t, err, domain.CodeSyntaxError)
}

func TestEvaluateRPN_EvaluatesValidStreams(t *testing.T) {
	cases := []struct {
		name     string
		rpn      []parser.Token
		expected float64
	}{
		{"single number", []parser.Token{num(42)}, 42},
		{"addition", []parser.Token{num(1), num(2), op("+")}, 3},
		{"precedence encoded in rpn", []parser.Token{num(10), num(20), num(3), op("*"), op("+")}, 70},
		{"unary negation", []parser.Token{num(5), op(unaryMinus)}, -5},
		{"function", []parser.Token{num(16), fn("sqrt")}, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.EvaluateRPN(tc.rpn)
			require.NoError(t, err)
			assert.InDelta(t, tc.expected, result, 1e-9)
		})
	}
}

func TestEvaluateRPN_RejectsMalformedStreams(t *testing.T) {
	cases := []struct {
		name string
		rpn  []parser.Token
	}{
		{"binary operator without operands", []parser.Token{op("+")}},
		{"binary operator with one operand", []parser.Token{num(1), op("+")}},
		{"unary operator without operand", []parser.Token{op(unaryMinus)}},
		{"function without argument", []parser.Token{fn("sqrt")}},
		{"leftover operands", []parser.Token{num(1), num(2)}},
		{"empty stream", []parser.Token{}},
		{"parenthesis leaked into rpn", []parser.Token{{Type: parser.TokenLeftParen, Value: "("}}},
		{"unknown operator", []parser.Token{num(1), num(2), op("&")}},
		// An unknown function has arity 0, which must not be allowed to pop
		// from an empty stack.
		{"unknown function on empty stack", []parser.Token{fn("log")}},
		{"unknown function with operand", []parser.Token{num(1), fn("log")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.EvaluateRPN(tc.rpn)
			requireAppError(t, err, domain.CodeSyntaxError)
		})
	}
}

func TestEvaluateRPN_PropagatesArithmeticErrors(t *testing.T) {
	_, err := parser.EvaluateRPN([]parser.Token{num(10), num(0), op("/")})
	requireAppError(t, err, domain.CodeDivisionByZero)

	_, err = parser.EvaluateRPN([]parser.Token{num(-1), fn("sqrt")})
	requireAppError(t, err, domain.CodeNegativeSqrt)

	_, err = parser.EvaluateRPN([]parser.Token{num(5), op(unaryMinus), num(0), op("/")})
	requireAppError(t, err, domain.CodeDivisionByZero)
}

func TestEvaluateRPN_RoundsFloatingPointArtifacts(t *testing.T) {
	result, err := parser.EvaluateRPN([]parser.Token{num(0.1), num(0.2), op("+")})

	require.NoError(t, err)
	assert.Equal(t, 0.3, result, "IEEE-754 artifacts must be rounded away")
}

func TestEnsureFinite(t *testing.T) {
	value, err := parser.EnsureFinite(42)
	require.NoError(t, err)
	assert.Equal(t, 42.0, value)

	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err = parser.EnsureFinite(invalid)
		requireAppError(t, err, domain.CodeNumericOverflow)
	}
}

func TestRound_RemovesArtifactsWithoutPerturbingExactValues(t *testing.T) {
	// Representation noise is collapsed.
	assert.Equal(t, 0.3, parser.Round(0.1+0.2))
	assert.Equal(t, 2.0, parser.Round(math.Sqrt(2)*math.Sqrt(2)))

	// Values that are already exact survive untouched, at every magnitude.
	for _, value := range []float64{
		0, 1, -7, 0.5, math.Pi,
		123456789012345.6, // 1e14: a decimal-place rounding scheme corrupts this
		9007199254740992,  // 2^53
		1e20, 1e-9, math.MaxFloat64,
	} {
		assert.Equal(t, value, parser.Round(value), "Round must not perturb %v", value)
	}

	assert.Equal(t, 0.0, parser.Round(-0.0), "negative zero is normalised")
	assert.True(t, math.IsNaN(parser.Round(math.NaN())))
	assert.True(t, math.IsInf(parser.Round(math.Inf(-1)), -1))
}

// TestEngine_PreservesPrecisionAtLargeMagnitudes guards the result
// normalisation against re-introducing the error it exists to remove.
func TestEngine_PreservesPrecisionAtLargeMagnitudes(t *testing.T) {
	cases := []struct {
		expression string
		expected   float64
	}{
		{"123456789012345.6 * 1", 123456789012345.6},
		{"0.1 + 0.2", 0.3},
		{"1 / 3 * 3", 1},
		{"9007199254740992 + 0", 9007199254740992},
	}

	engine := newEngine()
	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			evaluation, err := engine.Evaluate(tc.expression)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, evaluation.Result)
		})
	}
}

// -----------------------------------------------------------------------------
// Stage 5 - Formatting
// -----------------------------------------------------------------------------

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		name     string
		value    float64
		expected string
	}{
		{"integer", 65, "65"},
		{"negative integer", -7, "-7"},
		{"zero", 0, "0"},
		{"one decimal", 0.5, "0.5"},
		{"many decimals", 3.14159, "3.14159"},
		{"trailing zeros trimmed", 2.50, "2.5"},
		{"large magnitude switches to scientific", 1e20, "1e+20"},
		{"small magnitude switches to scientific", 1e-9, "1e-09"},
		{"nan", math.NaN(), "NaN"},
		{"positive infinity", math.Inf(1), "Infinity"},
		{"negative infinity", math.Inf(-1), "-Infinity"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, parser.FormatNumber(tc.value))
		})
	}
}

// TestFormatExpression_NormalisesSpacingAndSymbols pins the `formatted`
// response field: user spacing is normalised and operators are rendered with
// their typographic symbols.
func TestFormatExpression_NormalisesSpacingAndSymbols(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		expected   string
	}{
		{"tight input is spaced out", "10+20*3-15/(5-2)", "10 + 20 × 3 - 15 ÷ (5 - 2)"},
		{"loose input is compacted", "  10   *   (  2 +  3 )  ", "10 × (2 + 3)"},
		{"function renders as radical", "(10 + sqrt(16)) * 2^3", "(10 + √(16)) × 2 ^ 3"},
		{"leading sign hugs its operand", "-10 + sqrt(16) * 2", "-10 + √(16) × 2"},
		{"sign after operator hugs its operand", "10*-5", "10 × -5"},
		{"unary plus is preserved", "+10 - 5", "+10 - 5"},
		{"modulo keeps its symbol", "100%7", "100 % 7"},
		{"nested parentheses", "((1+2)*3)", "((1 + 2) × 3)"},
		{"numbers are canonicalised", "2.50 + 3.0", "2.5 + 3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, parser.FormatExpression(mustTokenize(t, tc.expression)))
		})
	}
}

func TestFormatExpression_EmptyStream(t *testing.T) {
	assert.Equal(t, "", parser.FormatExpression(nil))
}

// -----------------------------------------------------------------------------
// Stage 6 - Engine (full pipeline)
// -----------------------------------------------------------------------------

func TestEngine_EvaluatesWithPemdasPrecedence(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		expected   float64
	}{
		// The headline assessment scenarios.
		{"assessment complex expression", "10 + 20 * 3 - 15 / (5 - 2)", 65},
		{"assessment sqrt and power", "(10 + sqrt(16)) * 2^3", 112},
		{"assessment unary and function", "-10 + sqrt(16) * 2", -2},
		{"multiplication before addition", "10 + 20 * 3", 70},
		{"parentheses override", "(10 + 20) * 3", 90},

		// Basic operations.
		{"addition", "1 + 2", 3},
		{"subtraction", "10 - 4", 6},
		{"multiplication", "6 * 7", 42},
		{"division", "84 / 2", 42},
		{"decimals", "0.1 + 0.2", 0.3},
		{"negative result", "3 - 10", -7},

		// Advanced operations.
		{"power", "2 ^ 10", 1024},
		{"power right associative", "2 ^ 3 ^ 2", 512},
		{"fractional power", "9 ^ 0.5", 3},
		{"square root", "sqrt(144)", 12},
		{"nested square root", "sqrt(sqrt(16))", 2},
		{"modulo", "10 % 3", 1},
		{"modulo precedence", "10 + 10 % 3", 11},
		{"negative modulo", "-10 % 3", -1},

		// Signs and chains.
		{"leading unary minus", "-5 + 10", 5},
		{"unary minus with power", "-2 ^ 2", -4},
		{"unary in parentheses", "(-2) ^ 2", 4},
		{"unary after operator", "10 * -5", -50},
		{"negation after subtraction", "5 - -3", 8},
		{"negation after addition", "5 + -3", 2},
		{"negative exponent", "2 ^ -2", 0.25},
		{"long additive chain", "1 + 2 + 3 + 4 + 5 + 6 + 7 + 8 + 9 + 10", 55},
		{"deeply nested", "((((1 + 2) * 3) - 4) / 5)", 1},
		{"whitespace tolerant", "   10   *    (  2 +  3 )   ", 50},
		{"no whitespace", "10*(2+3)", 50},
		{"mixed everything", "2 ^ 3 + sqrt(25) * 2 - 10 % 4", 16},
	}

	engine := newEngine()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evaluation, err := engine.Evaluate(tc.expression)
			require.NoError(t, err)
			assert.InDelta(t, tc.expected, evaluation.Result, 1e-9)
		})
	}
}

func TestEngine_ReturnsNormalisedExpressionAndRPN(t *testing.T) {
	evaluation, err := newEngine().Evaluate("10+20*3-15/(5-2)")

	require.NoError(t, err)
	assert.Equal(t, 65.0, evaluation.Result)
	assert.Equal(t, "10 + 20 × 3 - 15 ÷ (5 - 2)", evaluation.Normalized)
	assert.Equal(t, "10 20 3 * + 15 5 2 - / -", strings.Join(tokenValues(evaluation.RPN), " "))
}

func TestEngine_PropagatesEvaluationErrors(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		code       string
	}{
		{"division by zero", "1 / 0", domain.CodeDivisionByZero},
		{"division by zero in sub-expression", "10 + 20 * 3 - 15 / (5 - 5)", domain.CodeDivisionByZero},
		{"modulo by zero", "10 % 0", domain.CodeDivisionByZero},
		{"negative square root", "sqrt(0 - 16)", domain.CodeNegativeSqrt},
		{"overflow", "9999 ^ 9999", domain.CodeNumericOverflow},
		{"undefined power", "(0 - 8) ^ 0.5", domain.CodeNumericOverflow},
	}

	engine := newEngine()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Evaluate(tc.expression)
			appErr := requireAppError(t, err, tc.code)
			assert.Equal(t, 400, appErr.Status)
		})
	}
}

func TestEngine_PropagatesSanitizerAndTokenizerErrors(t *testing.T) {
	engine := parser.NewEngine(20, 3)

	cases := []struct {
		name       string
		expression string
		code       string
	}{
		{"too long", strings.Repeat("1+", 20) + "1", domain.CodeExpressionTooLong},
		{"too deep", "((((1))))", domain.CodeNestingTooDeep},
		{"invalid character", "1 & 2", domain.CodeInvalidCharacter},
		{"injection attempt", "eval('1+1')", domain.CodeInvalidCharacter},
		{"unbalanced parentheses", "(1 + 2", domain.CodeSyntaxError},
		{"double operator", "1 ++ 2", domain.CodeSyntaxError},
		{"trailing operator", "1 +", domain.CodeSyntaxError},
		{"empty", "", domain.CodeValidationError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Evaluate(tc.expression)
			requireAppError(t, err, tc.code)
		})
	}
}

// TestNewEngine_FallsBackToDefaults verifies the fallback behaviourally: an
// engine built with invalid limits must enforce the documented defaults of
// 500 characters and 20 nesting levels.
func TestNewEngine_FallsBackToDefaults(t *testing.T) {
	engine := parser.NewEngine(0, -1)

	atLengthLimit := strings.Repeat("1+", 249) + "1" // 499 characters
	_, err := engine.Evaluate(atLengthLimit)
	require.NoError(t, err)

	overLengthLimit := strings.Repeat("1+", 300) + "1" // 601 characters
	_, err = engine.Evaluate(overLengthLimit)
	requireAppError(t, err, domain.CodeExpressionTooLong)

	atDepthLimit := strings.Repeat("(", 20) + "1" + strings.Repeat(")", 20)
	_, err = engine.Evaluate(atDepthLimit)
	require.NoError(t, err)

	overDepthLimit := strings.Repeat("(", 21) + "1" + strings.Repeat(")", 21)
	_, err = engine.Evaluate(overDepthLimit)
	requireAppError(t, err, domain.CodeNestingTooDeep)
}

// TestEngine_IsSafeForConcurrentUse guards the assumption made by the HTTP
// layer: a single engine instance is shared across all request goroutines.
func TestEngine_IsSafeForConcurrentUse(t *testing.T) {
	engine := newEngine()
	const goroutines = 32

	results := make(chan float64, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			evaluation, err := engine.Evaluate("10 + 20 * 3 - 15 / (5 - 2)")
			if err != nil {
				results <- -1
				return
			}
			results <- evaluation.Result
		}()
	}

	for i := 0; i < goroutines; i++ {
		assert.Equal(t, 65.0, <-results)
	}
}

// BenchmarkEngineEvaluate tracks the hot path: a complex nested expression must
// stay well under the sub-millisecond budget.
func BenchmarkEngineEvaluate(b *testing.B) {
	engine := newEngine()
	expression := "10 + 20 * 3 - 15 / (5 - 2) + sqrt(144) * 2 ^ 3 - 100 % 7"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Evaluate(expression); err != nil {
			b.Fatal(err)
		}
	}
}
