#!/bin/bash
# check-external-usage.sh - Check if exported symbols are used outside the package

PACKAGE_PATH="internal/parser/sysvar"

# Get all exported symbols from the package
echo "Checking exported symbols in $PACKAGE_PATH..."
echo "================================================"

# Use gopls to find all symbols, then filter for exported ones
gopls workspace_symbol "" | grep "$PACKAGE_PATH/[^/]*\.go:" | grep -v test | while read -r line; do
    # Extract symbol name and type
    location=$(echo "$line" | awk -F'[ :]' '{print $1":"$2":"$3}')
    symbol_name=$(echo "$line" | awk '{print $2}')
    symbol_type=$(echo "$line" | awk '{print $3}')
    
    # Skip if not exported (doesn't start with uppercase)
    if [[ ! "$symbol_name" =~ ^[A-Z] ]]; then
        continue
    fi
    
    # Skip certain symbol types that might be internal
    if [[ "$symbol_type" == "Field" ]] || [[ "$symbol_type" == "Method" ]]; then
        continue
    fi
    
    # Find references
    refs=$(gopls references "$location" 2>/dev/null | grep -v "$PACKAGE_PATH" | grep -v "_test.go" | wc -l)
    
    if [ "$refs" -eq 0 ]; then
        echo "UNUSED: $symbol_name ($symbol_type)"
    else
        echo "USED: $symbol_name ($symbol_type) - $refs external references"
    fi
done | sort