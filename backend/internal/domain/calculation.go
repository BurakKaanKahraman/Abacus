package domain

import "strings"

// Operation names accepted by the structured (N-operand array) payload.
const (
	OpAdd        = "add"
	OpSubtract   = "subtract"
	OpMultiply   = "multiply"
	OpDivide     = "divide"
	OpPower      = "power"
	OpModulo     = "modulo"
	OpSquareRoot = "sqrt"
)

// operationSymbols maps an operation name onto the operator lexeme understood
// by the expression engine. Aliases keep the API forgiving without widening the
// grammar.
//
// `%` is the modulo operator. "percentage" is deliberately not an alias for it:
// a caller asking for a percentage would silently receive a remainder, so an
// unsupported-operation error is the honest answer.
var operationSymbols = map[string]string{
	OpAdd:      "+",
	"plus":     "+",
	OpSubtract: "-",
	"minus":    "-",
	OpMultiply: "*",
	"times":    "*",
	OpDivide:   "/",
	"division": "/",
	OpPower:    "^",
	"pow":      "^",
	"exponent": "^",
	OpModulo:   "%",
	"mod":      "%",
}

// SupportedOperations lists the canonical operation names, in the order shown
// to API consumers.
var SupportedOperations = []string{
	OpAdd, OpSubtract, OpMultiply, OpDivide, OpPower, OpModulo, OpSquareRoot,
}

// OperatorFor resolves an operation name to its operator lexeme. The second
// return value reports whether the name is a known binary operation.
func OperatorFor(operation string) (string, bool) {
	symbol, ok := operationSymbols[strings.ToLower(strings.TrimSpace(operation))]
	return symbol, ok
}

// IsSquareRootOperation reports whether the operation name refers to the unary
// square root.
func IsSquareRootOperation(operation string) bool {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case OpSquareRoot, "squareroot", "square_root", "root":
		return true
	default:
		return false
	}
}

// CalculateRequest is the request payload of POST /api/v1/calculate. Clients
// send either a raw `expression` or an `operation` plus an `operands` array.
type CalculateRequest struct {
	Expression string    `json:"expression,omitempty"`
	Operation  string    `json:"operation,omitempty"`
	Operands   []float64 `json:"operands,omitempty"`
}

// CalculateResponse is the success payload of POST /api/v1/calculate.
type CalculateResponse struct {
	Expression string  `json:"expression"`
	Result     float64 `json:"result"`
	Formatted  string  `json:"formatted"`
	Timestamp  string  `json:"timestamp"`
}

// CalculationResult is the transport-agnostic outcome produced by the
// calculator usecase.
type CalculationResult struct {
	// Expression is the normalised expression that was evaluated.
	Expression string
	// Result is the evaluated value.
	Result float64
	// Formatted is the `expression = result` rendering.
	Formatted string
}

// HealthResponse is the payload of GET /api/v1/health.
type HealthResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// TokenResponse is the payload of POST /api/v1/auth/token.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ProblemDetails is an RFC 7807 error document, extended with a stable
// machine-readable `code` and a `timestamp`.
type ProblemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
	// Instance identifies the request that produced the problem, as defined by
	// RFC 7807. It carries the request path.
	Instance  string `json:"instance,omitempty"`
	Timestamp string `json:"timestamp"`
}
