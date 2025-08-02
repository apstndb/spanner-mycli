#!/bin/bash
# gopls-find-refs.sh - Find all references to a symbol

if [ $# -eq 0 ]; then
    echo "Usage: $0 <symbol_name>"
    echo "Finds all references to a symbol"
    exit 1
fi

SYMBOL="$1"

# First find the symbol definition
LOCATION=$(gopls workspace_symbol -matcher casesensitive "$SYMBOL" | \
    grep -E "(Interface|Struct|Function|Variable|Constant)$" | \
    head -1 | \
    awk -F'[ :]' '{print $1":"$2":"$3}')

if [ -z "$LOCATION" ]; then
    echo "Symbol '$SYMBOL' not found"
    exit 1
fi

echo "Symbol definition: $LOCATION"
echo "References:"
echo "----------------------------------------"

# Find all references
gopls references "$LOCATION" | \
    awk -F':' '{
        file=$1
        line=$2
        col=$3
        
        # Extract relative path
        gsub(/.*\/worktrees\/[^\/]+\//, "", file)
        
        printf "%s:%s:%s\n", file, line, col
    }' | sort -u