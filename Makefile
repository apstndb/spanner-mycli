build:
	go build

generate:
	go generate ./...

build-tools:
	# Install development tools using Go 1.24 tool management
	# gh-helper: Generic GitHub operations (reviews, threads) - managed via go.mod tool directive
	@echo "📦 Installing tools from go.mod tool directive..."
	go install tool
	@echo "✅ Tools installed successfully"
	@echo "💡 Use: go tool gh-helper --help"

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

# Test with coverage profile
test-coverage:
	@mkdir -p tmp
	@echo "🧪 Running tests with coverage..."
	@go test -coverpkg=./... ./... -coverprofile=tmp/coverage.out.tmp
	@echo "🔧 Excluding generated files from coverage..."
	@grep -v '_enumer\.go' tmp/coverage.out.tmp > tmp/coverage.out || true
	@rm -f tmp/coverage.out.tmp
	@echo "📊 Generating coverage report..."
	@go tool cover -html=tmp/coverage.out -o tmp/coverage.html
	@echo "📈 Coverage summary:"
	@go tool cover -func=tmp/coverage.out | tail -1
	@echo "✅ Coverage report generated: tmp/coverage.html"

# Open coverage report in browser
test-coverage-open: test-coverage
	@echo "🌐 Opening coverage report in browser..."
	@if command -v open >/dev/null 2>&1; then \
		open tmp/coverage.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open tmp/coverage.html; \
	else \
		echo "⚠️  Please open tmp/coverage.html manually"; \
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


# Enhanced development targets (issue #301 - script reorganization)
# Development targets using Go 1.24 tool management and simple Makefile workflows
.PHONY: test-quick test-coverage test-coverage-open check all all-quick docs-update help-dev worktree-setup worktree-list worktree-delete gh-review build-tools fmt fmt-check

# Quick tests for development cycle
test-quick:
	go test -short ./...

# Combined test, lint, and format check (required before push)
check: test lint fmt-check

# All development tasks with formatting (destructive)
all: fmt check

# Quick development cycle with formatting (destructive)
all-quick: fmt test-quick lint


# Update README.md help sections (replacing spanner-mycli-dev)
docs-update:
	@echo "📝 Updating help output for README.md..."
	@mkdir -p tmp
	@script -q tmp/help_output.txt sh -c "stty cols 200; go run . --help"
	@go run . --statement-help > tmp/statement_help.txt
	@sed '1s/^...//' tmp/help_output.txt > tmp/help_clean.txt
	@echo "✅ Help output files generated successfully in ./tmp/"
	@echo "📋 Generated files:"
	@echo "   - help_clean.txt: --help output for README.md"
	@echo "   - statement_help.txt: --statement-help output for README.md"

# Show development help
help-dev:
	@echo "🛠️  Development Commands:"
	@echo "  make build            - Build the application"
	@echo "  make build-tools      - Install gh-helper using Go 1.24 tool management"
	@echo "  make test             - Run full test suite (required before push)"
	@echo "  make test-coverage    - Run tests with coverage report"
	@echo "  make test-coverage-open - Run coverage and open HTML report in browser"
	@echo "  make test-quick       - Run quick tests (go test -short)"
	@echo "  make lint             - Run linter (required before push)"
	@echo "  make fmt              - Format code with gofmt, goimports, and gofumpt (⚠️ modifies files)"
	@echo "  make fmt-check        - Check if code is properly formatted"
	@echo "  make check            - Run test && lint && fmt-check (required before push)"
	@echo "  make all              - Run fmt && check (⚠️ modifies files)"
	@echo "  make all-quick        - Run fmt && test-quick && lint (⚠️ modifies files)"
	@echo "  make clean            - Clean build artifacts and test cache"
	@echo "  make run              - Run with PROJECT/INSTANCE/DATABASE env vars"
	@echo "  make docs-update      - Generate help output for README.md"
	@echo "  make worktree-setup   - Setup phantom worktree (requires WORKTREE_NAME)"
	@echo "  make worktree-list    - List existing phantom worktrees"
	@echo "  make worktree-delete  - Delete phantom worktree (requires WORKTREE_NAME)"
	@echo ""
	@echo "🔧 Development Tools:"
	@echo "  go tool gh-helper     - GitHub operations (managed via go.mod tool directive)"
	@echo ""
	@echo "🚀 Quick Start for AI Assistants:"
	@echo "  gh pr create && go tool gh-helper reviews wait  # Create PR + wait for review"
	@echo "  go tool gh-helper reviews wait <PR> --request-review  # Request Gemini review + wait"

# Phantom worktree management (replacing spanner-mycli-dev)
worktree-setup:
	@if [ -z "$(WORKTREE_NAME)" ]; then \
		echo "❌ WORKTREE_NAME required. Usage: make worktree-setup WORKTREE_NAME=issue-123-feature"; \
		exit 1; \
	fi
	@echo "🔧 Creating phantom worktree: $(WORKTREE_NAME)"
	@git fetch origin
	@phantom create $(WORKTREE_NAME) --base origin/main --exec "ln -sf ../../../../.claude .claude"
	@echo "✅ Worktree created successfully!"
	@echo "📋 Next: phantom shell $(WORKTREE_NAME) --tmux-horizontal"

worktree-list:
	@phantom list

worktree-delete:
	@if [ -z "$(WORKTREE_NAME)" ]; then \
		echo "❌ WORKTREE_NAME required. Usage: make worktree-delete WORKTREE_NAME=issue-123-feature"; \
		exit 1; \
	fi
	@echo "🔍 Checking status of worktree: $(WORKTREE_NAME)"
	@phantom exec $(WORKTREE_NAME) git status --porcelain
	@echo "🗑️  Deleting worktree: $(WORKTREE_NAME)"
	@phantom delete $(WORKTREE_NAME)
	@echo "✅ Worktree deleted successfully"

# GitHub review monitoring (requires PR_NUMBER)
gh-review:
	@if [ -z "$(PR_NUMBER)" ]; then \
		echo "❌ PR_NUMBER required. Usage: make gh-review PR_NUMBER=123"; \
		exit 1; \
	fi
	@go tool gh-helper reviews analyze $(PR_NUMBER) --timeout 15m
