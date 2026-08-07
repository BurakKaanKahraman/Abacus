package parser

// Engine is the expression evaluation pipeline:
//
//	sanitize -> tokenize -> validate sequence -> shunting-yard (RPN) -> evaluate
//
// It is stateless and therefore safe for concurrent use by multiple goroutines.
type Engine struct {
	maxLength int
	maxDepth  int
}

// Evaluation is the outcome of a successful evaluation.
type Evaluation struct {
	// Normalized is the canonical rendering of the input expression.
	Normalized string
	// Result is the evaluated value, rounded to the engine's precision.
	Result float64
	// RPN is the Reverse Polish Notation form, exposed for diagnostics.
	RPN []Token
}

// NewEngine builds an Engine. Non-positive limits fall back to the defaults.
func NewEngine(maxLength, maxDepth int) *Engine {
	if maxLength <= 0 {
		maxLength = DefaultMaxExpressionLength
	}
	if maxDepth <= 0 {
		maxDepth = DefaultMaxNestingDepth
	}
	return &Engine{maxLength: maxLength, maxDepth: maxDepth}
}

// Evaluate runs the full pipeline over a raw, untrusted expression string.
func (e *Engine) Evaluate(expression string) (Evaluation, error) {
	sanitized, err := Sanitize(expression, e.maxLength, e.maxDepth)
	if err != nil {
		return Evaluation{}, err
	}

	tokens, err := Tokenize(sanitized)
	if err != nil {
		return Evaluation{}, err
	}

	rpn, err := ToRPN(tokens)
	if err != nil {
		return Evaluation{}, err
	}

	result, err := EvaluateRPN(rpn)
	if err != nil {
		return Evaluation{}, err
	}

	return Evaluation{
		Normalized: FormatExpression(tokens),
		Result:     result,
		RPN:        rpn,
	}, nil
}
