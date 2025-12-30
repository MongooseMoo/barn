# Barn - Go MOO Server Makefile

.PHONY: build clean test run conformance help

# Default target
all: build

# Build the main barn executable
build:
	go build -o barn.exe ./cmd/barn/

# Build with race detector (for debugging)
build-race:
	go build -race -o barn-race.exe ./cmd/barn/

# Clean build artifacts
clean:
	rm -f barn.exe barn-race.exe barn_test.exe
	rm -f server_*.log test_*.log output_*.log

# Run Go tests
test:
	go test ./...

# Run Go tests with verbose output
test-v:
	go test -v ./...

# Start server on default port (7777)
run: build
	./barn.exe -db Test.db -port 7777

# Start server on test port (9300)
run-test: build
	./barn.exe -db Test.db -port 9300

# Run conformance tests against running server (requires server on port 9300)
conformance:
	cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "not multiple_writes" -q

# Run conformance tests with verbose output
conformance-v:
	cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "not multiple_writes" -v

# Run conformance tests stopping on first failure
conformance-x:
	cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "not multiple_writes" -x -v

# Run specific conformance test category (usage: make conformance-k K=limits)
conformance-k:
	cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "$(K)" -v

# Quick manual test
quick-test: build
	./barn.exe -db Test.db -port 9300 > server.log 2>&1 & \
	sleep 2 && \
	printf 'connect wizard\n; return 1 + 1;\n' | nc -w 3 localhost 9300

# Help
help:
	@echo "Barn Makefile targets:"
	@echo "  build          - Build barn.exe"
	@echo "  build-race     - Build with race detector"
	@echo "  clean          - Remove build artifacts and logs"
	@echo "  test           - Run Go unit tests"
	@echo "  test-v         - Run Go unit tests (verbose)"
	@echo "  run            - Start server on port 7777"
	@echo "  run-test       - Start server on port 9300"
	@echo "  conformance    - Run conformance tests (quiet)"
	@echo "  conformance-v  - Run conformance tests (verbose)"
	@echo "  conformance-x  - Run conformance tests (stop on first fail)"
	@echo "  conformance-k  - Run specific tests (K=pattern)"
	@echo "  quick-test     - Build and run quick sanity test"
