# Streaming Output

## Overview

Streaming output reduces memory usage and improves response time for large query results by processing rows incrementally instead of buffering the entire result set.

## Configuration

### System Variable

```sql
-- Automatic selection based on format (default)
SET CLI_STREAMING = AUTO;

-- Force streaming for all formats
SET CLI_STREAMING = TRUE;

-- Disable streaming (always use buffered mode) 
SET CLI_STREAMING = FALSE;
```

### Command-line Flag

```bash
spanner-mycli --streaming=AUTO   # Default: automatic selection
spanner-mycli --streaming=TRUE   # Force streaming
spanner-mycli --streaming=FALSE  # Force buffered mode
```

## Default Behavior (AUTO mode)

| Format | Default Mode | Reason |
|--------|-------------|---------|
| CSV, Tab, Vertical | Streaming | Low memory usage, immediate output |
| HTML, XML | Streaming | Structure allows incremental output |
| Table formats | Buffered | Accurate column width calculation |

## Table Format Streaming

For table formats, a preview mechanism is used:

```sql
-- Control the number of rows used for width calculation (default: 50)
SET CLI_TABLE_PREVIEW_ROWS = 100;
```

The preview collects the specified number of rows to calculate optimal column widths, then streams the remaining rows using those widths.