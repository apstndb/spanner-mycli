# Streaming Output

## Overview

Streaming output is a feature that reduces Time To First Byte (TTFB) when executing queries with large result sets. Instead of buffering all rows in memory before displaying them, streaming mode processes and outputs rows as they arrive from the database.

## Benefits

- **Reduced TTFB**: First results appear immediately after the query starts returning rows
- **Lower memory usage**: Only a small buffer is needed instead of storing entire result set
- **Better user experience**: Users see progress immediately on large queries
- **Scalability**: Can handle result sets of any size without memory constraints

## Configuration

### Enable Streaming

```sql
-- Enable streaming for the current session
SET CLI_STREAMING_ENABLED = TRUE;

-- Disable streaming (default)
SET CLI_STREAMING_ENABLED = FALSE;
```

### Table Format Configuration

For table format output, streaming uses a preview-based approach where it buffers the first N rows to calculate optimal column widths:

```sql
-- Set number of preview rows for table width calculation
-- Default: 50 rows
-- 0: Use header widths only
-- Positive: Use that many rows for preview
-- -1: Collect all rows (non-streaming behavior)
SET CLI_TABLE_PREVIEW_ROWS = 100;
```

## Supported Formats

All output formats support streaming:

| Format | Streaming Behavior | Notes |
|--------|-------------------|-------|
| CSV | Direct streaming | Headers output immediately, rows streamed |
| Tab | Direct streaming | Headers output immediately, rows streamed |
| Vertical | Direct streaming | Each row formatted and output immediately |
| HTML | Direct streaming | Table rows streamed within HTML structure |
| XML | Direct streaming | Rows streamed within XML structure |
| Table | Preview-based | First N rows buffered for width calculation |

## How It Works

### Architecture

1. **Row Iterator Conversion**: `spanner.RowIterator` is converted to `iter.Seq[Row]` using the `rowIterToSeq` function
2. **Metadata Extraction**: Metadata is captured after the first row is fetched
3. **Row Processing**: Rows are processed through a `RowProcessor` interface that handles formatting
4. **Direct Output**: Formatted rows are written directly to the output stream

### Table Format Special Handling

Table format requires column width calculation for proper alignment. The streaming implementation uses a two-phase approach:

1. **Preview Phase**: Buffer first N rows (configured by `CLI_TABLE_PREVIEW_ROWS`)
2. **Calculate Widths**: Determine optimal column widths based on preview rows
3. **Stream Remaining**: Output all rows (including preview) with calculated widths

## Performance Characteristics

### Time To First Byte (TTFB)

- **Buffered Mode**: TTFB = Query execution time + Time to fetch all rows
- **Streaming Mode**: TTFB = Query execution time + Time to fetch first row

For large result sets, streaming can reduce TTFB from seconds/minutes to milliseconds.

### Memory Usage

- **Buffered Mode**: O(n) where n = total number of rows
- **Streaming Mode**: 
  - Direct formats (CSV, Tab, etc.): O(1) - constant memory
  - Table format: O(p) where p = preview rows (default 50)

## Debug and Monitoring

Enable debug logging to see streaming behavior:

```bash
./spanner-mycli --log-level=DEBUG
```

Key debug messages:
- `"Using streaming mode"` - Streaming is active
- `"Metadata retrieved (TTFB)"` - First row received, metadata available
- `"Using buffered mode"` - Streaming is disabled

## Examples

### Basic Usage

```sql
-- Enable streaming
SET CLI_STREAMING_ENABLED = TRUE;

-- Large query will start outputting immediately
SELECT * FROM large_table;
```

### CSV Export with Streaming

```sql
SET CLI_FORMAT = CSV;
SET CLI_STREAMING_ENABLED = TRUE;

-- Export large table to CSV without buffering
SELECT * FROM products ORDER BY created_at;
```

### Table Format with Custom Preview

```sql
SET CLI_FORMAT = TABLE;
SET CLI_STREAMING_ENABLED = TRUE;
SET CLI_TABLE_PREVIEW_ROWS = 200;  -- Use more rows for width calculation

SELECT product_name, description, price FROM products;
```

## Limitations

1. **Table format width calculation**: Column widths are determined from preview rows. Very wide values appearing after the preview may be truncated or wrapped.

2. **Progress indicators**: Some progress indicators (like row count) may not be available until the stream completes.

3. **Query plan and stats**: These are only available after all rows have been processed.

## Implementation Details

The streaming implementation maximizes code reuse between buffered and streaming modes:

- Unified row iteration using `iter.Seq[Row]`
- Common `RowProcessor` interface for all formatters
- Shared formatting logic in formatter implementations

Key components:
- `streaming.go`: Core streaming infrastructure
- `streaming_processors.go`: Streaming processor implementations
- `formatters_*.go`: Format-specific streaming formatters
- `row_processor.go`: RowProcessor interface definition