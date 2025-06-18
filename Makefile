build:
	go build

build-tools:
	# Build development tools from issue #301 script reorganization
	# gh-helper: Generic GitHub operations (reviews, threads)
	# spanner-mycli-dev: Project-specific workflows (worktrees, docs, Gemini integration)
	mkdir -p bin
	cd dev-tools/gh-helper && go build -o ../../bin/gh-helper .
	cd dev-tools/spanner-mycli-dev && go build -o ../../bin/spanner-mycli-dev .

clean:
	rm -f spanner-mycli
	rm -rf dist/
	rm -rf bin/
	go clean -testcache

run:
	./spanner-mycli -p ${PROJECT} -i ${INSTANCE} -d ${DATABASE}

test:
	go test ./...

test-verbose:
	go test -v ./...

# Test with coverage profile (for CI)
test-coverage:
	go test ./... -coverprofile=coverage.out

# Test both main project and dev-tools (comprehensive testing)
test-all:
	go test ./...
	cd dev-tools && go test ./...

# Test both main project and dev-tools with coverage (for CI)
test-all-coverage:
	go test ./... -coverprofile=coverage.out
	cd dev-tools && go test ./... -coverprofile=coverage-dev-tools.out

lint:
	golangci-lint run

# Lint both main project and dev-tools (comprehensive linting)
lint-all:
	golangci-lint run
	cd dev-tools && golangci-lint run

# Enhanced development targets (issue #301 - AI-friendly script reorganization)
# These targets integrate the new Go-based development tools (gh-helper, spanner-mycli-dev)
# replacing scattered shell scripts with structured, maintainable commands.
.PHONY: test-quick check docs-update help-dev worktree-setup gh-review build-tools

# Quick tests for development cycle
test-quick:
	go test -short ./...

# Combined test and lint check (required before push)
check: test lint

# Combined test and lint check for both main project and dev-tools (full validation)
check-all: test-all lint-all

# Combined test with coverage and lint check for both main project and dev-tools (CI validation)
check-all-coverage: test-all-coverage lint-all

# Update README.md help sections
docs-update:
	@bin/spanner-mycli-dev docs update-help

# Show development help
help-dev:
	@echo "🛠️  Development Commands:"
	@echo "  make build            - Build the application"
	@echo "  make build-tools      - Build gh-helper and spanner-mycli-dev tools"
	@echo "  make test             - Run full test suite (required before push)"
	@echo "  make test-coverage    - Run tests with coverage profile (for CI)"
	@echo "  make test-all         - Run tests for main project and dev-tools"
	@echo "  make test-all-coverage - Run tests with coverage for main project and dev-tools (for CI)"
	@echo "  make test-quick       - Run quick tests (go test -short)"
	@echo "  make lint             - Run linter (required before push)"
	@echo "  make lint-all         - Run linter for main project and dev-tools"
	@echo "  make check            - Run test && lint (required before push)"
	@echo "  make check-all        - Run test-all && lint-all (comprehensive check)"
	@echo "  make check-all-coverage - Run test-all-coverage && lint-all (for CI)"
	@echo "  make clean          - Clean build artifacts and test cache"
	@echo "  make run            - Run with PROJECT/INSTANCE/DATABASE env vars"
	@echo "  make docs-update    - Generate help output for README.md"
	@echo "  make worktree-setup - Setup phantom worktree (requires WORKTREE_NAME)"
	@echo "  make gh-review      - Check PR reviews (requires PR_NUMBER)"
	@echo ""
	@echo "🔧 Development Tools (issue #301 - AI-friendly script reorganization):"
	@echo "  bin/gh-helper       - Generic GitHub operations (reviews, threads)"
	@echo "  bin/spanner-mycli-dev - Project-specific tools (worktrees, docs, Gemini)"
	@echo ""
	@echo "🚀 Quick Start for AI Assistants:"
	@echo "  bin/gh-helper reviews wait <PR> --request-review  # Complete review workflow"
	@echo "  bin/spanner-mycli-dev pr-workflow create --wait-checks  # Full PR creation"

# Phantom worktree setup (requires WORKTREE_NAME)
worktree-setup:
	@if [ -z "$(WORKTREE_NAME)" ]; then \
		echo "❌ WORKTREE_NAME required. Usage: make worktree-setup WORKTREE_NAME=issue-123-feature"; \
		exit 1; \
	fi
	@bin/spanner-mycli-dev worktree setup $(WORKTREE_NAME)

# GitHub review monitoring (requires PR_NUMBER)
gh-review:
	@if [ -z "$(PR_NUMBER)" ]; then \
		echo "❌ PR_NUMBER required. Usage: make gh-review PR_NUMBER=123"; \
		exit 1; \
	fi
	@bin/gh-helper reviews check $(PR_NUMBER)
