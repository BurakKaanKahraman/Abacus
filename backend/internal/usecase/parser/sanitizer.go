package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// Sanitizer limits and structural guards. They exist to stop denial-of-service
// attempts (huge or deeply nested input) before any parsing work happens.
const (
	// DefaultMaxExpressionLength caps the raw expression string length.
	DefaultMaxExpressionLength = 500
	// DefaultMaxNestingDepth caps how deeply parentheses may nest.
	DefaultMaxNestingDepth = 20
)

// identifierPattern matches any run of letters/underscores. Every match must
// resolve to a whitelisted function name.
var identifierPattern = regexp.MustCompile(`[A-Za-z_]+`)

// Sanitize validates the raw expression against the character whitelist, the
// length budget and the parenthesis structure. It returns the trimmed
// expression ready for tokenization.
//
// Sanitize is the security boundary of the engine: it runs before the
// tokenizer so that hostile input never reaches the evaluation stack.
func Sanitize(expression string, maxLength, maxDepth int) (string, error) {
	if maxLength <= 0 {
		maxLength = DefaultMaxExpressionLength
	}
	if maxDepth <= 0 {
		maxDepth = DefaultMaxNestingDepth
	}

	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return "", domain.NewValidationError("Expression must not be empty.")
	}

	if runeLen := len([]rune(trimmed)); runeLen > maxLength {
		return "", domain.NewExpressionTooLongError(fmt.Sprintf(
			"Expression length %d exceeds the maximum of %d characters.", runeLen, maxLength))
	}

	if err := checkIdentifiers(trimmed); err != nil {
		return "", err
	}
	if err := checkCharacters(trimmed); err != nil {
		return "", err
	}
	if err := checkParentheses(trimmed, maxDepth); err != nil {
		return "", err
	}

	return trimmed, nil
}

// checkIdentifiers rejects any alphabetic token that is not a known function.
// This is what turns `eval(...)`, `DROP TABLE` or `__import__` into a 400
// instead of an unknown token deeper in the pipeline.
func checkIdentifiers(expression string) error {
	for _, loc := range identifierPattern.FindAllStringIndex(expression, -1) {
		name := expression[loc[0]:loc[1]]
		if !IsFunction(strings.ToLower(name)) {
			return domain.NewInvalidCharacterError(fmt.Sprintf(
				"Unsupported identifier %q at position %d. Only the sqrt function is allowed.",
				name, runePosition(expression, loc[0])))
		}
	}
	return nil
}

// checkCharacters applies the whitelist to everything that is not part of a
// recognised identifier. isAllowedRune is the single source of truth: a regex
// mirror of it would drift, since RE2's \s class is ASCII-only.
func checkCharacters(expression string) error {
	stripped := identifierPattern.ReplaceAllStringFunc(expression, func(s string) string {
		return strings.Repeat(" ", len(s))
	})
	for i, r := range stripped {
		if !isAllowedRune(r) {
			return domain.NewInvalidCharacterError(fmt.Sprintf(
				"Invalid character %q at position %d. Allowed characters are digits, '.', '+', '-', '*', '/', '^', '%%', '(', ')' and sqrt.",
				r, runePosition(expression, i)))
		}
	}
	return nil
}

func isAllowedRune(r rune) bool {
	if isSpace(r) || (r >= '0' && r <= '9') {
		return true
	}
	switch r {
	case '.', '+', '-', '*', '/', '^', '%', '(', ')':
		return true
	default:
		return false
	}
}

// isSpace recognises ASCII whitespace only. Exotic Unicode spaces (non-breaking
// space, line separator) are rejected rather than silently skipped, so that
// invisible characters can never alter how an expression parses.
func isSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	default:
		return false
	}
}

// checkParentheses verifies that parentheses are balanced and that nesting
// stays within the configured depth, guarding the evaluator against
// stack exhaustion.
func checkParentheses(expression string, maxDepth int) error {
	depth := 0
	for i, r := range expression {
		switch r {
		case '(':
			depth++
			if depth > maxDepth {
				return domain.NewNestingTooDeepError(fmt.Sprintf(
					"Parenthesis nesting depth exceeds the maximum of %d levels.", maxDepth))
			}
		case ')':
			depth--
			if depth < 0 {
				return domain.NewSyntaxError(fmt.Sprintf(
					"Unbalanced parentheses in expression: %q. Unexpected ')' at position %d.",
					expression, runePosition(expression, i)))
			}
		}
	}
	if depth > 0 {
		return domain.NewSyntaxError(fmt.Sprintf(
			"Unbalanced parentheses in expression: %q. Missing %d closing ')'.", expression, depth))
	}
	return nil
}

// runePosition converts a byte offset into a 1-based rune position so that
// error messages stay meaningful for multi-byte input.
func runePosition(s string, byteOffset int) int {
	if byteOffset <= 0 {
		return 1
	}
	if byteOffset > len(s) {
		byteOffset = len(s)
	}
	return len([]rune(s[:byteOffset])) + 1
}
