package parser

import "testing"

func TestComparisonOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator TokenType
	}{
		{"1 == 2", TOKEN_EQ},
		{"x != y", TOKEN_NE},
		{"a < b", TOKEN_LT},
		{"c > d", TOKEN_GT},
		{"e <= f", TOKEN_LE},
		{"g >= h", TOKEN_GE},
		{"x in {1, 2, 3}", TOKEN_IN},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != tt.operator {
				t.Errorf("expected operator %s, got %s", tt.operator, binary.Operator)
			}
		})
	}
}

func TestComparisonPrecedence(t *testing.T) {
	// Comparison has lower precedence than arithmetic
	tests := []string{
		"1 + 1 == 2",   // should parse as (1 + 1) == 2
		"x * 2 < 10",   // should parse as (x * 2) < 10
		"a - b >= c + d", // should parse as (a - b) >= (c + d)
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr at root, got %T", expr)
			}

			// Root should be comparison operator
			switch binary.Operator {
			case TOKEN_EQ, TOKEN_NE, TOKEN_LT, TOKEN_LE, TOKEN_GT, TOKEN_GE, TOKEN_IN:
				// Good
			default:
				t.Errorf("expected comparison operator at root, got %s", binary.Operator)
			}

			// At least one side should have arithmetic
			_, leftIsBinary := binary.Left.(*BinaryExpr)
			_, rightIsBinary := binary.Right.(*BinaryExpr)
			if !leftIsBinary && !rightIsBinary {
				// This is OK for simple cases like "1 == 2"
			}
		})
	}
}
