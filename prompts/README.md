# Spec Audit Workflow

Two-agent loop for finding and fixing specification gaps.

## The Problem

Specs written by someone who knows the system often have implicit knowledge baked in. A "blind implementor" (someone with only the spec) would hit gaps where they'd have to guess.

## The Workflow

```
┌─────────────────────┐
│ blind-implementor-  │
│ audit.md            │
│                     │
│ Input: Feature name │
│ Access: spec/ ONLY  │
│ Output: reports/    │
│   audit-{feature}.md│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ spec-patcher.md     │
│                     │
│ Input: reports/     │
│   audit-{feature}.md│
│ Access: ALL code    │
│ Output: reports/    │
│   patch-{feature}.md│
└──────────┬──────────┘
           │
           ▼
     Patched spec/
```

## File Communication

Agents communicate via files in `reports/`:
- `audit-*.md` - Gap reports from blind implementor
- `patch-*.md` - Resolution reports from patcher

Agents return only SHORT SUMMARIES to orchestrator. Full details in files.

## Usage

### Step 1: Audit a Feature

```
Subagent prompt: @prompts/blind-implementor-audit.md

Audit the "for loop" feature. Write to reports/audit-for-loop.md
```

Agent writes full report to file, returns summary.

### Step 2: Patch the Gaps

```
Subagent prompt: @prompts/spec-patcher.md

Patch gaps in reports/audit-for-loop.md. Write to reports/patch-for-loop.md
```

Agent reads audit file, researches, patches specs, writes resolution to file, returns summary.

### Step 3: Iterate

Repeat for each major feature:
- Statements: if, for, while, fork, try/except, try/finally
- Operators: each precedence level, short-circuit, catch expression
- Types: each type's coercion rules, edge cases
- Builtins: each category
- Object model: inheritance, properties, verbs
- Task model: fork, suspend, resume, kill

## Feature Audit Order

Suggested order (foundational → complex):

1. **Types** - INT, FLOAT, STR edge cases
2. **Operators** - Arithmetic, comparison, logical
3. **Lists** - Indexing, slicing, mutation
4. **Maps** - Keys, iteration, mutation
5. **Statements** - if, for, while
6. **Exceptions** - try/except, try/finally, catch expr
7. **Objects** - create, properties, verbs
8. **Inheritance** - parent lookup, multiple inheritance
9. **Tasks** - fork, suspend, resume

## Gap Categories

- **guess**: Spec doesn't say at all
- **assume**: Spec implies but doesn't state explicitly
- **ask**: Would need to ask someone who knows
- **test**: Would need to run existing code to find out

## Success Criteria

A spec passes the blind implementor test when:
- Zero gaps of type "guess" or "ask"
- All "assume" gaps made explicit
- All edge cases documented
- Error conditions exhaustively listed
