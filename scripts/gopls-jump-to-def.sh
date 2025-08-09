#!/bin/bash
# gopls-jump-to-def.sh - Find symbol and jump to its definition

if [ $# -eq 0 ]; then
    echo "Usage: $0 <symbol_name>"
    echo "Finds a symbol and shows its definition"
    exit 1
fi

SYMBOL="$1"

# Find the symbol location
LOCATION=$(gopls workspace_symbol "$SYMBOL" | head -1 | awk -F'[ :]' '{print $1":"$2":"$3}')

if [ -z "$LOCATION" ]; then
    echo "Symbol '$SYMBOL' not found"
    exit 1
fi

echo "Found symbol at: $LOCATION"
echo "----------------------------------------"

# Show the definition
gopls definition "$LOCATION"