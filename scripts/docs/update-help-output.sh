#!/bin/bash
set -euo pipefail

# Update help output in README.md
# This script automates the process documented in CLAUDE.md for updating help sections

echo "📝 Updating help output files for README.md..."

# Create working directory
mkdir -p ./tmp

# Generate --help output with proper column width
echo "Generating --help output..."
if ! script -q ./tmp/help_output.txt sh -c "stty cols 200; go run . --help"; then
    echo "❌ Failed to generate --help output"
    exit 1
fi

# Generate --statement-help output
echo "Generating --statement-help output..."
if ! go run . --statement-help > ./tmp/statement_help.txt; then
    echo "❌ Failed to generate --statement-help output"
    exit 1
fi

# Clean up help output format (remove first 2 characters from script output)
echo "Cleaning up output format..."
sed '1s/^.\{2\}//' ./tmp/help_output.txt > ./tmp/help_clean.txt

# Verify generated files exist and have content
if [[ ! -s ./tmp/help_clean.txt ]]; then
    echo "❌ Generated help_clean.txt is empty"
    exit 1
fi

if [[ ! -s ./tmp/statement_help.txt ]]; then
    echo "❌ Generated statement_help.txt is empty"
    exit 1
fi

echo "✅ Help output files generated successfully in ./tmp/"
echo ""
echo "📋 Generated files:"
echo "   - help_clean.txt: --help output for README.md"
echo "   - statement_help.txt: --statement-help output for README.md"
echo ""
echo "⚠️  Manual step required:"
echo "   Update README.md with generated content from the files above"
echo ""
echo "💡 Future enhancement:"
echo "   Consider adding automatic README.md update functionality"

# Files are left in ./tmp/ for manual integration
# Future version could automatically update README.md sections