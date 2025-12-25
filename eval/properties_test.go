package eval

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"testing"
)

func TestPropertyInheritance(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	// Create parent object with a property
	parent := db.NewObject(0, 0)
	parent.Properties["name"] = &db.Property{
		Name:  "name",
		Value: types.NewStr("parent_name"),
		Owner: 0,
		Perms: db.PropRead | db.PropWrite,
		Clear: false,
	}
	store.Add(parent)

	// Create child object inheriting from parent
	child := db.NewObject(1, 0)
	child.Parents = []types.ObjID{0}
	// Child has name property but it's clear (inherits)
	child.Properties["name"] = &db.Property{
		Name:  "name",
		Value: nil,
		Owner: 1,
		Perms: db.PropRead | db.PropWrite,
		Clear: true, // Inherits from parent
	}
	store.Add(child)

	// Test: child.name should return parent's value
	propExpr := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(1)},
		Property: "name",
	}

	result := eval.Eval(propExpr, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property access failed: %v", result)
	}

	strVal, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("Expected StrValue, got %T", result.Val)
	}

	if strVal.String() != `"parent_name"` {
		t.Errorf("Expected inherited value \"parent_name\", got %s", strVal.String())
	}
}

func TestPropertyOverride(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	// Create parent object
	parent := db.NewObject(0, 0)
	parent.Properties["name"] = &db.Property{
		Name:  "name",
		Value: types.NewStr("parent_name"),
		Owner: 0,
		Perms: db.PropRead | db.PropWrite,
		Clear: false,
	}
	store.Add(parent)

	// Create child object that overrides the property
	child := db.NewObject(1, 0)
	child.Parents = []types.ObjID{0}
	child.Properties["name"] = &db.Property{
		Name:  "name",
		Value: types.NewStr("child_name"),
		Owner: 1,
		Perms: db.PropRead | db.PropWrite,
		Clear: false, // Has its own value
	}
	store.Add(child)

	// Test: child.name should return child's value
	propExpr := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(1)},
		Property: "name",
	}

	result := eval.Eval(propExpr, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property access failed: %v", result)
	}

	strVal, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("Expected StrValue, got %T", result.Val)
	}

	if strVal.String() != `"child_name"` {
		t.Errorf("Expected override value \"child_name\", got %s", strVal.String())
	}
}

func TestMultipleInheritance(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	// Create grandparent with property x
	grandparent := db.NewObject(0, 0)
	grandparent.Properties["x"] = &db.Property{
		Name:  "x",
		Value: types.NewInt(100),
		Owner: 0,
		Perms: db.PropRead | db.PropWrite,
		Clear: false,
	}
	store.Add(grandparent)

	// Create parent1 (inherits from grandparent, has property y)
	parent1 := db.NewObject(1, 0)
	parent1.Parents = []types.ObjID{0}
	parent1.Properties["y"] = &db.Property{
		Name:  "y",
		Value: types.NewInt(200),
		Owner: 1,
		Perms: db.PropRead | db.PropWrite,
		Clear: false,
	}
	store.Add(parent1)

	// Create parent2 (independent, has property z)
	parent2 := db.NewObject(2, 0)
	parent2.Properties["z"] = &db.Property{
		Name:  "z",
		Value: types.NewInt(300),
		Owner: 2,
		Perms: db.PropRead | db.PropWrite,
		Clear: false,
	}
	store.Add(parent2)

	// Create child with two parents
	child := db.NewObject(3, 0)
	child.Parents = []types.ObjID{1, 2} // parent1, parent2
	store.Add(child)

	// Test: child should inherit x from grandparent through parent1
	propX := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(3)},
		Property: "x",
	}

	result := eval.Eval(propX, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property x access failed: %v", result)
	}

	if intVal, ok := result.Val.(types.IntValue); !ok || intVal.Val != 100 {
		t.Errorf("Expected x=100, got %v", result.Val)
	}

	// Test: child should inherit y from parent1
	propY := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(3)},
		Property: "y",
	}

	result = eval.Eval(propY, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property y access failed: %v", result)
	}

	if intVal, ok := result.Val.(types.IntValue); !ok || intVal.Val != 200 {
		t.Errorf("Expected y=200, got %v", result.Val)
	}

	// Test: child should inherit z from parent2
	propZ := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(3)},
		Property: "z",
	}

	result = eval.Eval(propZ, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property z access failed: %v", result)
	}

	if intVal, ok := result.Val.(types.IntValue); !ok || intVal.Val != 300 {
		t.Errorf("Expected z=300, got %v", result.Val)
	}
}

func TestDiamondInheritance(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	//     D(x=100)
	//     /     \
	//   B(x=200) C(clear)
	//     \     /
	//       A
	// A should inherit x=200 from B (left parent checked first)

	// Create D (top)
	d := db.NewObject(3, 0)
	d.Properties["x"] = &db.Property{
		Name:  "x",
		Value: types.NewInt(100),
		Clear: false,
	}
	store.Add(d)

	// Create B (inherits from D, overrides x)
	b := db.NewObject(1, 0)
	b.Parents = []types.ObjID{3}
	b.Properties["x"] = &db.Property{
		Name:  "x",
		Value: types.NewInt(200),
		Clear: false,
	}
	store.Add(b)

	// Create C (inherits from D, clears x)
	c := db.NewObject(2, 0)
	c.Parents = []types.ObjID{3}
	c.Properties["x"] = &db.Property{
		Name:  "x",
		Value: nil,
		Clear: true,
	}
	store.Add(c)

	// Create A (multiple parents: B and C)
	a := db.NewObject(0, 0)
	a.Parents = []types.ObjID{1, 2} // B, C
	store.Add(a)

	// Test: A.x should be 200 (from B, first parent)
	propExpr := &parser.PropertyExpr{
		Pos:      parser.Position{Line: 1, Column: 1},
		Expr:     &parser.LiteralExpr{Value: types.NewObj(0)},
		Property: "x",
	}

	result := eval.Eval(propExpr, ctx)
	if !result.IsNormal() {
		t.Fatalf("Property access failed: %v", result)
	}

	intVal, ok := result.Val.(types.IntValue)
	if !ok {
		t.Fatalf("Expected IntValue, got %T", result.Val)
	}

	if intVal.Val != 200 {
		t.Errorf("Expected x=200 (from B), got %d", intVal.Val)
	}
}
