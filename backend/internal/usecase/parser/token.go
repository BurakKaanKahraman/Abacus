// Package parser implements the expression engine: a hand written tokenizer,
// a Shunting-Yard infix -> RPN converter and a stack based RPN evaluator.
//
// It deliberately avoids third party expression libraries and never evaluates
// user input as code, which removes an entire class of injection risk.
package parser

// TokenType enumerates the lexical categories produced by the tokenizer.
type TokenType int

const (
	// TokenNumber is a numeric literal such as `10` or `3.14`.
	TokenNumber TokenType = iota
	// TokenOperator is a binary or unary operator such as `+` or `u-`.
	TokenOperator
	// TokenFunction is a named function such as `sqrt`.
	TokenFunction
	// TokenLeftParen is `(`.
	TokenLeftParen
	// TokenRightParen is `)`.
	TokenRightParen
)

// Internal lexemes used for unary operators. They can never appear in user
// input (the whitelist rejects letters outside function names), so they are
// unambiguous markers.
const (
	unaryMinus = "u-"
	unaryPlus  = "u+"
)

// Token is a single lexical unit with its source position for error reporting.
type Token struct {
	Type   TokenType
	Value  string  // the lexeme, e.g. "+", "sqrt", "10.5"
	Number float64 // populated for TokenNumber only
	Pos    int     // 1-based rune position in the source expression
}

// IsUnary reports whether the token is a unary operator.
func (t Token) IsUnary() bool {
	return t.Type == TokenOperator && (t.Value == unaryMinus || t.Value == unaryPlus)
}

// operatorInfo carries the precedence table entry for an operator.
type operatorInfo struct {
	precedence int
	rightAssoc bool
	unary      bool
}

// operators is the PEMDAS/BODMAS precedence table.
//
//	5  ^          (right associative)
//	4  unary + -  (right associative)
//	3  * / %      (left associative)
//	2  + -        (left associative)
//
// Functions and parentheses bind tighter than every operator and are handled
// structurally by the Shunting-Yard algorithm rather than via this table.
var operators = map[string]operatorInfo{
	"^":        {precedence: 5, rightAssoc: true},
	unaryMinus: {precedence: 4, rightAssoc: true, unary: true},
	unaryPlus:  {precedence: 4, rightAssoc: true, unary: true},
	"*":        {precedence: 3},
	"/":        {precedence: 3},
	"%":        {precedence: 3},
	"+":        {precedence: 2},
	"-":        {precedence: 2},
}

// functions is the whitelist of callable functions and their arity.
var functions = map[string]int{
	"sqrt": 1,
}

// IsFunction reports whether name is a supported function.
func IsFunction(name string) bool {
	_, ok := functions[name]
	return ok
}
