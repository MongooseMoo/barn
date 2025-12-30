package main

import (
	"barn/parser"
	"fmt"
)

func main() {
	code := "return `args ! ANY => 0';"
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
		return
	}
	fmt.Printf("Parsed %d statements\n", len(stmts))
	for i, stmt := range stmts {
		fmt.Printf("Statement %d: %T\n", i, stmt)
	}
}
