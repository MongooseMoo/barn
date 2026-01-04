# Task: Detect Divergences in Property Builtins

## Context

We need to verify Barn's property builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all property builtins.

## Files to Read

- `spec/builtins/properties.md` - expected behavior specification
- `builtins/properties.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Property Access
- Direct property access: `obj.prop`
- `property_value()` - get property value by name
- `set_property_value()` - set property value by name

### Property Existence/Info
- `properties()` - list defined properties
- `property_info()` - get {owner, perms} for property
- `set_property_info()` - modify owner/perms
- `is_clear_property()` - check if property is cleared

### Property Definition
- `add_property()` - create new property with value, owner, perms
- `delete_property()` - remove property definition
- `clear_property()` - clear inherited value

### Property Permissions
- "r" - read permission
- "w" - write permission
- "c" - chown permission
- Permission inheritance from parent

### Inheritance Behavior
- Property defined on parent, accessed on child
- Clearing vs setting on child
- is_clear_property() semantics

## Edge Cases to Test

- Built-in properties (name, location, owner, etc.)
- Properties on #0 (system object)
- Non-existent properties
- Permission violations (E_PERM)
- Invalid property names
- Property inheritance chain
- Properties on recycled objects

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe '#0.name'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return #0.name;"

# Check conformance tests
grep -r "property_info\|add_property\|properties\|clear_property" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-properties.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major property builtin
- Pay special attention to inheritance and permission semantics
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
