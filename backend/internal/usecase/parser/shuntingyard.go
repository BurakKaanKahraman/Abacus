package parser

import (
	"fmt"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
)

// ToRPN converts an infix token stream into Reverse Polish Notation using
// Dijkstra's Shunting-Yard algorithm, applying the PEMDAS/BODMAS precedence
// table and operator associativity.
//
// RPN removes the need for a recursive descent parser, so evaluation runs on an
// explicit stack in O(n) time with no recursion depth risk.
func ToRPN(tokens []Token) ([]Token, error) {
	output := make([]Token, 0, len(tokens))
	stack := make([]Token, 0, len(tokens))

	for _, tok := range tokens {
		switch tok.Type {
		case TokenNumber:
			output = append(output, tok)

		case TokenFunction:
			stack = append(stack, tok)

		case TokenOperator:
			info, ok := operators[tok.Value]
			if !ok {
				return nil, domain.NewSyntaxError(fmt.Sprintf(
					"Unknown operator %q at position %d.", tok.Value, tok.Pos))
			}
			// A unary operator is a prefix operator: its operand has not been
			// read yet, so it never pops anything. This is what keeps
			// `2 ^ -3` (2^(-3)) and `-2 ^ 2` (-(2^2)) both correct.
			if info.unary {
				stack = append(stack, tok)
				continue
			}
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				if top.Type == TokenLeftParen {
					break
				}
				if top.Type == TokenFunction {
					output = append(output, top)
					stack = stack[:len(stack)-1]
					continue
				}
				topInfo := operators[top.Value]
				higher := topInfo.precedence > info.precedence
				equalLeftAssoc := topInfo.precedence == info.precedence && !info.rightAssoc
				if !higher && !equalLeftAssoc {
					break
				}
				output = append(output, top)
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, tok)

		case TokenLeftParen:
			stack = append(stack, tok)

		case TokenRightParen:
			matched := false
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.Type == TokenLeftParen {
					matched = true
					break
				}
				output = append(output, top)
			}
			if !matched {
				return nil, domain.NewSyntaxError(fmt.Sprintf(
					"Unbalanced parentheses: unexpected ')' at position %d.", tok.Pos))
			}
			// A function call is popped together with its closing parenthesis.
			if len(stack) > 0 && stack[len(stack)-1].Type == TokenFunction {
				output = append(output, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
		}
	}

	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.Type == TokenLeftParen {
			return nil, domain.NewSyntaxError(fmt.Sprintf(
				"Unbalanced parentheses: missing ')' for '(' at position %d.", top.Pos))
		}
		output = append(output, top)
	}

	return output, nil
}
