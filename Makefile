build:
	go build

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

# Test with coverage profile (for CI)
test-coverage:
	@mkdir -p tmp
	go test ./... -coverprofile=tmp/coverage.out


lint:
	golangci-lint run


# Enhanced development targets (issue #301 - script reorganization)
# Development targets using Go 1.24 tool management and simple Makefile workflows
.PHONY: test-quick check docs-update help-dev worktree-setup worktree-list worktree-delete gh-review build-tools

# Quick tests for development cycle
test-quick:
	go test -short ./...

# Combined test and lint check (required before push)
check: test lint


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
	@echo "  make test-coverage    - Run tests with coverage profile (for CI)"
	@echo "  make test-quick       - Run quick tests (go test -short)"
	@echo "  make lint             - Run linter (required before push)"
	@echo "  make check            - Run test && lint (required before push)"
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
