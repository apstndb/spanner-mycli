#!/bin/bash
# gopls-find-symbol.sh - Find symbol locations using gopls

if [ $# -eq 0 ]; then
    echo "Usage: $0 <symbol_name> [matcher]"
    echo "Matchers: fuzzy, fastfuzzy, casesensitive, caseinsensitive (default)"
    exit 1
fi

SYMBOL="$1"
MATCHER="${2:-caseinsensitive}"

# Search for symbol and format output
gopls workspace_symbol -matcher "$MATCHER" "$SYMBOL" | \
    awk -F'[ :]' '{
        file=$1
        line=$2
        col_start=$3
        col_end=substr($4, 1, index($4, " ")-1)
        symbol_name=$5
        symbol_type=$6
        
        # Extract relative path
        gsub(/.*\/worktrees\/[^\/]+\//, "", file)
        
        printf "%-60s %s:%s:%s %s\n", symbol_name " (" symbol_type ")", file, line, col_start, ""
    }'