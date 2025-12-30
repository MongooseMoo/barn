# Task: Research How to Run ToastStunt with toastcore.db

## Context

We need to run ToastStunt (C++ MOO server) as our reference/oracle implementation to compare Barn's behavior against. ToastStunt source is at `~/src/toaststunt/`.

## Objective

Figure out how to:
1. Build ToastStunt (if not already built)
2. Run it with toastcore.db on a specific port
3. Document the exact commands needed

## Research Steps

1. Check `~/src/toaststunt/` for existing binaries
2. Look for README, INSTALL, or build instructions
3. Check for existing database files (toastcore.db or similar)
4. Figure out command-line arguments for port selection and database

## Output

Write findings to `./reports/research-run-toaststunt.md` with:
- Exact commands to build (if needed)
- Exact command to run ToastStunt with a database on a given port
- Location of toastcore.db (or how to obtain it)
- Any gotchas or requirements

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
