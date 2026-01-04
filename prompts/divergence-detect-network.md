# Task: Detect Divergences in Network Builtins

## Context

We need to verify Barn's network builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all network builtins.

## Files to Read

- `spec/builtins/network.md` - expected behavior specification
- `builtins/network.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### DNS/Host Lookups
- `name_lookup()` - resolve hostname to IP
- `dns_lookup()` - DNS queries

### URL Handling
- `parse_url()` - parse URL components
- `encode_url()` - URL encoding
- `decode_url()` - URL decoding

### HTTP
- `http_fetch()` - HTTP requests (if exists)

## Edge Cases to Test

- Invalid hostnames
- IPv4 vs IPv6
- Special characters in URLs
- Malformed URLs
- Connection timeouts

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'name_lookup("localhost")'
./toast_oracle.exe 'parse_url("http://example.com/path?q=1")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return name_lookup(\"localhost\");"

# Check conformance tests
grep -r "name_lookup\|parse_url\|encode_url\|decode_url" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-network.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- These may be ToastStunt-only extensions
- Do NOT make external network requests that could be blocked
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
