#!/bin/bash
set -euo pipefail

# Automated phantom worktree setup with Claude settings
# Automates the workflow documented in CLAUDE.md

# Function to show usage
show_usage() {
    echo "Usage: $0 <issue-number-description>"
    echo ""
    echo "Examples:"
    echo "  $0 issue-276-timeout-flag"
    echo "  $0 issue-284-ellipsis-feature"
    echo "  $0 fix-lint-warnings"
    echo ""
    echo "This script will:"
    echo "  1. Fetch latest changes from origin"
    echo "  2. Create a phantom worktree based on origin/main"
    echo "  3. Set up Claude settings symlink"
    echo "  4. Provide next steps for development"
}

# Check arguments
if [ $# -ne 1 ]; then
    show_usage
    exit 1
fi

WORKTREE_NAME="$1"

# Validate worktree name format (basic validation)
if [[ ! "$WORKTREE_NAME" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo "❌ Invalid worktree name. Use only letters, numbers, hyphens, and underscores."
    exit 1
fi

echo "🔧 Creating phantom worktree: $WORKTREE_NAME"

# Check if phantom command exists
if ! command -v phantom &> /dev/null; then
    echo "❌ phantom command not found. Please install phantom first:"
    echo "   https://github.com/aku11i/phantom"
    exit 1
fi

# Check if worktree already exists
if phantom list 2>/dev/null | grep -q "^$WORKTREE_NAME\$"; then
    echo "❌ Worktree '$WORKTREE_NAME' already exists"
    echo ""
    echo "📋 Existing worktrees:"
    phantom list
    exit 1
fi

# Fetch latest changes from origin
echo "Fetching latest changes from origin..."
if ! git fetch origin; then
    echo "❌ Failed to fetch from origin"
    exit 1
fi

# Create phantom worktree based on origin/main with Claude settings
echo "Creating worktree and setting up Claude configuration..."
if ! phantom create "$WORKTREE_NAME" --base origin/main --exec 'ln -sf ../../../../.claude .claude'; then
    echo "❌ Failed to create phantom worktree"
    exit 1
fi

echo "✅ Worktree created successfully!"
echo ""
echo "📋 Next steps:"
echo "   phantom shell $WORKTREE_NAME --tmux-horizontal"
echo ""
echo "💡 Remember to:"
echo "   - Record development insights in PR comments"
echo "   - Run 'make test && make lint' before push"
echo "   - Check 'phantom exec $WORKTREE_NAME git status' before cleanup"
echo ""
echo "🧹 Cleanup when done:"
echo "   phantom exec $WORKTREE_NAME git status  # Verify no uncommitted changes"
echo "   phantom delete $WORKTREE_NAME"