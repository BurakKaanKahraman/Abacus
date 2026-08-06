package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// Tokenize converts a sanitized infix expression into a token stream and
// validates the token sequence. Signs are classified as unary or binary here,
// where the preceding token is still in scope.
func Tokenize(expression string) ([]Token, error) {
	runes := []rune(expression)
	tokens := make([]Token, 0, len(runes)/2+1)

	for i := 0; i < len(runes); {
		r := runes[i]
		pos := i + 1

		switch {
		case isSpace(r):
			i++

		case isDigit(r) || r == '.':
			tok, next, err := readNumber(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next

		case unicode.IsLetter(r) || r == '_':
			tok, next, err := readIdentifier(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next

		case r == '(':
			tokens = append(tokens, Token{Type: TokenLeftParen, Value: "(", Pos: pos})
			i++

		case r == ')':
			tokens = append(tokens, Token{Type: TokenRightParen, Value: ")", Pos: pos})
			i++

		case r == '+' || r == '-':
			value, err := classifySign(tokens, r, pos)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{Type: TokenOperator, Value: value, Pos: pos})
			i++

		case isOperatorRune(r):
			tokens = append(tokens, Token{Type: TokenOperator, Value: string(r), Pos: pos})
			i++

		default:
			// Defensive: Sanitize should already have rejected this input.
			return nil, domain.NewInvalidCharacterError(fmt.Sprintf(
				"Invalid character %q at position %d.", r, pos))
		}
	}

	if len(tokens) == 0 {
		return nil, domain.NewValidationError("Expression must not be empty.")
	}
	if err := validateSequence(tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// readNumber consumes a numeric literal starting at index start.
func readNumber(runes []rune, start int) (Token, int, error) {
	i := start
	dots := 0
	for i < len(runes) && (isDigit(runes[i]) || runes[i] == '.') {
		if runes[i] == '.' {
			dots++
		}
		i++
	}
	literal := string(runes[start:i])
	if dots > 1 || literal == "." {
		return Token{}, 0, domain.NewSyntaxError(fmt.Sprintf(
			"Malformed number %q at position %d.", truncate(literal), start+1))
	}
	value, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		// A syntactically valid literal can still fall outside the float64
		// range; that is an overflow, not a syntax error.
		if errors.Is(err, strconv.ErrRange) {
			return Token{}, 0, domain.NewNumericOverflowError(fmt.Sprintf(
				"Number literal %q at position %d is outside the representable float64 range.",
				truncate(literal), start+1))
		}
		return Token{}, 0, domain.NewSyntaxError(fmt.Sprintf(
			"Malformed number %q at position %d.", truncate(literal), start+1))
	}
	return Token{Type: TokenNumber, Value: literal, Number: value, Pos: start + 1}, i, nil
}

// readIdentifier consumes a function name starting at index start.
func readIdentifier(runes []rune, start int) (Token, int, error) {
	i := start
	for i < len(runes) && (unicode.IsLetter(runes[i]) || runes[i] == '_') {
		i++
	}
	name := strings.ToLower(string(runes[start:i]))
	if !IsFunction(name) {
		return Token{}, 0, domain.NewInvalidCharacterError(fmt.Sprintf(
			"Unsupported identifier %q at position %d. Only the sqrt function is allowed.",
			string(runes[start:i]), start+1))
	}
	return Token{Type: TokenFunction, Value: name, Pos: start + 1}, i, nil
}

// classifySign decides whether a `+`/`-` is unary (sign) or binary
// (addition/subtraction) based on the previous token.
//
// Both signs are unary at the start of the expression and after `(`. After a
// binary operator only `-` may follow, as a negation of the right operand:
// that keeps mathematically meaningful input working (`5 - -3`, `10 * -5`,
// `2 ^ -3`) while `10 ++ 20` and `10 -+ 20` are reported as double operators.
// Two stacked signs are always rejected.
func classifySign(tokens []Token, r rune, pos int) (string, error) {
	unaryValue := unaryMinus
	if r == '+' {
		unaryValue = unaryPlus
	}

	if len(tokens) == 0 {
		return unaryValue, nil
	}

	prev := tokens[len(tokens)-1]
	switch {
	case prev.Type == TokenLeftParen:
		return unaryValue, nil
	case prev.Type == TokenOperator && prev.IsUnary():
		return "", domain.NewSyntaxError(fmt.Sprintf(
			"Double operator '%s%s' at position %d.", displayValue(prev), string(r), pos))
	case prev.Type == TokenOperator:
		if r == '-' {
			return unaryMinus, nil
		}
		return "", domain.NewSyntaxError(fmt.Sprintf(
			"Double operator '%s%s' at position %d.", displayValue(prev), string(r), pos))
	default:
		// Follows a number or ')': binary operator.
		return string(r), nil
	}
}

// truncate shortens a literal for error messages so that a hostile 500
// character input cannot inflate the response body.
func truncate(literal string) string {
	const maxLen = 32
	runes := []rune(literal)
	if len(runes) <= maxLen {
		return literal
	}
	return string(runes[:maxLen]) + "…"
}

// validateSequence walks the token stream with a two-state machine
// (operand expected / operator expected) and rejects every structurally
// impossible arrangement before the Shunting-Yard stage.
func validateSequence(tokens []Token) error {
	expectOperand := true

	for i, tok := range tokens {
		switch tok.Type {
		case TokenNumber:
			if !expectOperand {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unexpected number %q at position %d; an operator was expected.", tok.Value, tok.Pos))
			}
			expectOperand = false

		case TokenFunction:
			if !expectOperand {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unexpected function %q at position %d; an operator was expected.", tok.Value, tok.Pos))
			}
			if i+1 >= len(tokens) || tokens[i+1].Type != TokenLeftParen {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Function %q at position %d must be followed by '('.", tok.Value, tok.Pos))
			}

		case TokenLeftParen:
			if !expectOperand {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unexpected '(' at position %d; an operator was expected before it.", tok.Pos))
			}
			if i+1 < len(tokens) && tokens[i+1].Type == TokenRightParen {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Empty parentheses at position %d.", tok.Pos))
			}
			expectOperand = true

		case TokenRightParen:
			if expectOperand {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unexpected ')' at position %d; an operand was expected before it.", tok.Pos))
			}
			expectOperand = false

		case TokenOperator:
			if tok.IsUnary() {
				if !expectOperand {
					return domain.NewSyntaxError(fmt.Sprintf(
						"Unexpected sign %q at position %d.", tok.Value, tok.Pos))
				}
				continue // still expecting an operand
			}
			if expectOperand {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unexpected operator %q at position %d; an operand was expected.", tok.Value, tok.Pos))
			}
			expectOperand = true
		}
	}

	if expectOperand {
		last := tokens[len(tokens)-1]
		return domain.NewSyntaxError(fmt.Sprintf(
			"Expression ends unexpectedly after %q at position %d.", displayValue(last), last.Pos))
	}
	return nil
}

func displayValue(t Token) string {
	if t.IsUnary() {
		return strings.TrimPrefix(t.Value, "u")
	}
	return t.Value
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isOperatorRune(r rune) bool {
	switch r {
	case '+', '-', '*', '/', '^', '%':
		return true
	default:
		return false
	}
}
