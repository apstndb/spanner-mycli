#!/bin/sh
# Copyright 2026 apstndb
#
# Claude Code PostToolUse hook: auto-format a Go file after Edit/Write.
#
# Receives the hook payload as JSON on stdin, extracts .tool_input.file_path,
# and runs the repository formatter (golangci-lint fmt: gofumpt + goimports)
# on it when it is an existing .go file. Always exits 0 so a missing
# formatter or unexpected payload never blocks the edit itself; formatting
# problems are still caught later by `make check` / CI.

payload=$(cat)

if command -v jq >/dev/null 2>&1; then
    file_path=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
else
    # Fallback without jq: extract the first "file_path" string value with
    # POSIX sed only (grep -o is a GNU extension). Good enough for the plain
    # absolute paths Claude Code sends; paths with embedded escaped quotes
    # are not expected.
    file_path=$(printf '%s' "$payload" |
        sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
        head -n 1)
fi

[ -n "$file_path" ] || exit 0

case "$file_path" in
*.go) ;;
*) exit 0 ;;
esac

[ -f "$file_path" ] || exit 0

if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint fmt "$file_path" >/dev/null 2>&1 || true
fi

exit 0
