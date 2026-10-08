# Barn - Go MOO Server Makefile

.PHONY: all build build-linux-amd64 build-race clean test test-v run conformance-build conformance conformance-v conformance-x conformance-k conformance-toast help

# Binaries go to bin/ (gitignored); Go appends .exe on Windows.
BIN := bin
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)

# Managed conformance targets run inside Linux/WSL. Use a Linux-only uv environment.
CONFORMANCE_SUITE ?= ../moo-conformance-tests
CONFORMANCE_ENV ?= $(HOME)/.cache/barn-make-conformance-linux
CONFORMANCE_PATHS ?=
CONFORMANCE_ARGS ?=
TOAST_MOO ?= /root/src/toaststunt/build-release/moo
K ?=
CONFORMANCE_OUTPUT = -q

MANAGED_CONFORMANCE = UV_PROJECT_ENVIRONMENT="$(CONFORMANCE_ENV)" uv run --project "$(abspath $(CONFORMANCE_SUITE))" --frozen moo-conformance $(CONFORMANCE_PATHS) \
	-m "admission or conformance" \
	--server-db="$(abspath $(CONFORMANCE_SUITE))/src/moo_conformance/_db/Test.db" \
	--server-db-dir="$(abspath $(CONFORMANCE_SUITE))/src/moo_conformance/_db/startup" \
	--moo-host=127.0.0.1 \
	--oracle-profile-manifest="$(CURDIR)/profiles/toast/stock-wsl-testdb.json" \
	--fail-on-unexpected-skip --strict-markers \
	$(if $(strip $(K)),-k "capability_admission or ($(K))") $(CONFORMANCE_ARGS)

# Default target
all: build

# Build every command under cmd/ into bin/
build:
	go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
	go build -o $(BIN)/ ./cmd/...

# Build the optimized linux/amd64 deployment and bench_differ binary.
# Generic release artifacts intentionally retain Go's GOAMD64=v1 default.
build-linux-amd64:
	go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
	GOOS=linux GOARCH=amd64 GOAMD64=v3 CGO_ENABLED=0 go build -o $(BIN)/barn-linux-amd64 ./cmd/barn/

# Build with race detector (for debugging)
build-race:
	go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
	go build -race -o $(BIN)/barn-race$(EXE) ./cmd/barn/

# Clean build artifacts
clean:
	rm -rf -- "$(BIN)"

# Run Go tests
test:
	go test ./...

# Run Go tests with verbose output
test-v:
	go test -v ./...

# Start server on default port (7777)
run: build
	./$(BIN)/barn$(EXE) -db Test.db -port 7777

# Build only Barn for the managed server lifecycle; the harness copies fixtures.
conformance-build:
	go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
	go build -o "$(BIN)/barn$(EXE)" ./cmd/barn

conformance-v conformance-k: CONFORMANCE_OUTPUT = -v
conformance-x: CONFORMANCE_OUTPUT = -x -v

# K narrows any target while retaining canonical admission in the same session.
conformance conformance-v conformance-x conformance-k: conformance-build
	$(MANAGED_CONFORMANCE) \
		--server-command='"$(abspath $(BIN))/barn$(EXE)" --db {db} --listen tcp://127.0.0.1:{port} --config="$(CURDIR)/profiles/barn/outbound-on.conf" --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}' \
		$(CONFORMANCE_OUTPUT)

# Stock WSL Toast uses the same managed harness and its verified profile.
conformance-toast:
	test -x "$(TOAST_MOO)"
	$(MANAGED_CONFORMANCE) \
		--server-command='"$(TOAST_MOO)" {db} {db}.new -p {port}' \
		--target-profile-manifest="$(CURDIR)/profiles/toast/stock-wsl-testdb.json" -q

# Help
help:
	@echo "Barn Makefile targets:"
	@echo "  build          - Build all cmd/ tools into bin/"
	@echo "  build-linux-amd64 - Build v3 linux/amd64 deployment binary"
	@echo "  build-race     - Build with race detector"
	@echo "  clean          - Remove bin/ build artifacts (preserve root logs)"
	@echo "  test           - Run Go unit tests"
	@echo "  test-v         - Run Go unit tests (verbose)"
	@echo "  run            - Start server on port 7777"
	@echo "  conformance    - Run conformance tests (quiet)"
	@echo "  conformance-v  - Run conformance tests (verbose)"
	@echo "  conformance-x  - Run conformance tests (stop on first fail)"
	@echo "  conformance-k  - Run specific tests plus admission (K=pattern)"
	@echo "  conformance-toast - Run the stock WSL Toast managed oracle"
	@echo "Conformance runs inside Linux/WSL; CONFORMANCE_SUITE selects the harness checkout."
	@echo "K=pattern narrows any conformance target; CONFORMANCE_PATHS selects packaged files."
