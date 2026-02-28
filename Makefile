.PHONY: build generate clean run test test-verbose test-quick test-coverage test-coverage-open \
	lint fmt fmt-check check all all-quick docs-update help-dev \
	worktree-setup worktree-list worktree-delete

build:
	go build

generate:
	go generate ./...

clean:
	rm -f spanner-mycli
	rm -rf dist/
	go clean -testcache

run:
	./spanner-mycli -p ${PROJECT} -i ${INSTANCE} -d ${DATABASE}

test:
	go test ./...

test-verbose:
	go test -v ./...

# Quick tests for development cycle
test-quick:
	go test -short ./...

# Test with coverage profile
test-coverage:
	@mkdir -p tmp
	@echo "Running tests with coverage..."
	@go test -coverpkg=./... ./... -coverprofile=tmp/coverage.out.tmp
	@echo "Excluding generated files from coverage..."
	@grep -E -v '(_enumer\.go|enums/.*_enumer\.go)' tmp/coverage.out.tmp > tmp/coverage.out || true
	@rm -f tmp/coverage.out.tmp
	@echo "Generating coverage report..."
	@go tool cover -html=tmp/coverage.out -o tmp/coverage.html
	@echo "Coverage summary:"
	@go tool cover -func=tmp/coverage.out | tail -1
	@echo "Coverage report generated: tmp/coverage.html"

# Open coverage report in browser
test-coverage-open: test-coverage
	@if command -v open >/dev/null 2>&1; then \
		open tmp/coverage.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open tmp/coverage.html; \
	else \
		echo "Please open tmp/coverage.html manually"; \
	fi

lint:
	golangci-lint run

fmt:
	@echo "Formatting code..."
	golangci-lint fmt .
	@echo "Code formatted successfully"

fmt-check:
	@echo "Checking code formatting..."
	@if golangci-lint fmt --diff . | grep -q "^diff"; then \
		echo "Code formatting issues found. Run 'make fmt' to fix."; \
		golangci-lint fmt --diff . | head -100; \
		exit 1; \
	else \
		echo "Code formatting is correct"; \
	fi

# Combined test, lint, and format check (required before push)
check: test lint fmt-check

# All development tasks with formatting (destructive)
all: fmt check

# Quick development cycle with formatting (destructive)
all-quick: fmt test-quick lint

# Update README.md help sections
# Note: go-flags uses ioctl(TIOCGWINSZ) for terminal width, ignoring COLUMNS env var.
# script(1) creates a PTY so stty can set the width. macOS syntax; Linux differs.
docs-update:
	@echo "Updating help output for README.md..."
	@mkdir -p tmp
	@script -q tmp/help_output.txt sh -c "stty cols 200; go run . --help"
	@sed '1s/^...//' tmp/help_output.txt > tmp/help_clean.txt
	@go run . --statement-help > tmp/statement_help.txt
	@echo "Generated files:"
	@echo "  - tmp/help_clean.txt: --help output for README.md"
	@echo "  - tmp/statement_help.txt: --statement-help output for README.md"

# Show development help
help-dev:
	@echo "Development Commands:"
	@echo "  make build             - Build the application"
	@echo "  make test              - Run full test suite (required before push)"
	@echo "  make test-coverage     - Run tests with coverage report"
	@echo "  make test-coverage-open - Run coverage and open HTML report in browser"
	@echo "  make test-quick        - Run quick tests (go test -short)"
	@echo "  make lint              - Run linter (required before push)"
	@echo "  make fmt               - Format code (modifies files)"
	@echo "  make fmt-check         - Check if code is properly formatted"
	@echo "  make check             - Run test && lint && fmt-check (required before push)"
	@echo "  make all               - Run fmt && check (modifies files)"
	@echo "  make all-quick         - Run fmt && test-quick && lint (modifies files)"
	@echo "  make clean             - Clean build artifacts and test cache"
	@echo "  make run               - Run with PROJECT/INSTANCE/DATABASE env vars"
	@echo "  make docs-update       - Generate help output for README.md"
	@echo "  make worktree-setup    - Setup phantom worktree (requires WORKTREE_NAME)"
	@echo "  make worktree-list     - List existing phantom worktrees"
	@echo "  make worktree-delete   - Delete phantom worktree (requires WORKTREE_NAME)"

# Phantom worktree management
worktree-setup:
	@if [ -z "$(WORKTREE_NAME)" ]; then \
		echo "WORKTREE_NAME required. Usage: make worktree-setup WORKTREE_NAME=issue-123-feature"; \
		exit 1; \
	fi
	@echo "Creating phantom worktree: $(WORKTREE_NAME)"
	@git fetch origin
	@phantom create $(WORKTREE_NAME) --base origin/main
	@echo "Worktree created successfully!"
	@echo "Next: phantom shell $(WORKTREE_NAME) --tmux-horizontal"

worktree-list:
	@phantom list

worktree-delete:
	@if [ -z "$(WORKTREE_NAME)" ]; then \
		echo "WORKTREE_NAME required. Usage: make worktree-delete WORKTREE_NAME=issue-123-feature"; \
		exit 1; \
	fi
	@echo "Checking status of worktree: $(WORKTREE_NAME)"
	@phantom exec $(WORKTREE_NAME) git status --porcelain
	@echo "Deleting worktree: $(WORKTREE_NAME)"
	@phantom delete $(WORKTREE_NAME)
	@echo "Worktree deleted successfully"
