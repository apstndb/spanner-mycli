#!/bin/bash
# gopls-find-by-type.sh - Find symbols by type (Interface, Function, Struct, etc.)

if [ $# -lt 2 ]; then
    echo "Usage: $0 <symbol_pattern> <type>"
    echo "Types: Interface, Function, Struct, Method, Variable, Constant"
    echo "Example: $0 Parser Interface"
    exit 1
fi

PATTERN="$1"
TYPE="$2"

# Search and filter by type
gopls workspace_symbol "$PATTERN" | \
    grep " $TYPE$" | \
    awk -F'[ :]' '{
        file=$1
        line=$2
        col=$3
        symbol_info=$5" "$6
        
        # Extract relative path
        gsub(/.*\/worktrees\/[^\/]+\//, "", file)
        
        printf "%-50s %s:%s:%s\n", symbol_info, file, line, col
    }' | sort