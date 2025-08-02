#!/bin/bash
# find-unused-exports.sh - Find exported symbols that are not used outside their package

PACKAGE="internal/parser/sysvar"

# Get all exported symbols
echo "Checking exported symbols in $PACKAGE..."
echo "=========================================="

grep -E "^(type|func|var|const) [A-Z][a-zA-Z0-9_]*" $PACKAGE/*.go | grep -v test | while IFS=: read -r file declaration; do
    # Extract symbol name
    symbol=$(echo "$declaration" | sed -E 's/^(type|func|var|const) ([A-Z][a-zA-Z0-9_]*).*/\2/')
    
    if [ -z "$symbol" ]; then
        continue
    fi
    
    # Find references outside the package
    external_refs=$(grep -r "$symbol" --include="*.go" --exclude-dir="$PACKAGE" . 2>/dev/null | grep -v "_test.go" | wc -l)
    
    if [ "$external_refs" -eq 0 ]; then
        echo "UNUSED: $symbol (defined in $file)"
    fi
done