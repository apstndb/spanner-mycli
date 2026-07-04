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
    # Fallback without jq: extract a "file_path" JSON key value with POSIX
    # sed only (grep -o is a GNU extension). The key must be preceded by a
    # brace, comma, or whitespace so keys merely ending in file_path do not
    # match. This is best-effort: payload text inside old_string/new_string
    # can still false-match, but the .go suffix check, the -f check, and the
    # fact that the only action is running the formatter bound the impact to
    # formatting an unintended Go file, which is harmless. jq is the primary
    # path.
    file_path=$(printf '%s' "$payload" |
        sed -n 's/.*[[:space:],{]"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
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
