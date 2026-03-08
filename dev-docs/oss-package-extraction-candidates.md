# OSS Package Extraction Candidates

Analysis of internal packages that could be extracted as independent OSS packages.

## Already-Extracted Packages (Reference)

| Package | Purpose |
|---------|---------|
| `github.com/apstndb/spanvalue` | Format Spanner values to strings |
| `github.com/apstndb/spantype` | Format Spanner types |
| `github.com/apstndb/spannerplan` | Query plan visualization |
| `github.com/apstndb/go-runewidthex` | Unicode width calculation |
| `github.com/apstndb/lox` | Logging/utility functions |
| `github.com/apstndb/memebridge` | Spanner query plan bridging |
| `github.com/apstndb/spanemuboost` | Spanner emulator utilities |
| `github.com/apstndb/gsqlutils` | Google Cloud SQL utilities |
| `github.com/apstndb/adcplus` | ADC enhancement |
| `github.com/apstndb/go-grpcinterceptors` | gRPC middleware |

## Dependency Graph (Current)

```
┌─────────────────────────────────────────┐
│        external apstndb packages        │
│ (spanvalue, spantype, memefish, etc.)   │
└────────────────┬────────────────────────┘
                 │
    ┌────────────┴────────────┐
    │                         │
┌───▼────────┐         ┌──────▼────────┐
│   decoder  │         │   formatsql   │
│  (bridge)  │         │ (SQL export)  │
└────────────┘         └──────┬────────┘
                              │
                     ┌────────▼──────┐
                     │  format pkg   │
                     │ (formatters)  │
                     └───────────────┘

┌─────────────────────────────────┐
│ Zero-Dependency (Standalone):   │
│ - metrics                       │
│ - streamio                      │
│ - filesafety                    │
└─────────────────────────────────┘
```

---

## Tier 1: High-Priority Candidates

### 1. `format/` — Output Formatter Package

**Location:** `internal/mycli/format/`
**Size:** ~2,200 LOC (code + tests)

**What it does:**
- Multi-format tabular output (TABLE, CSV, TAB, VERTICAL, HTML, XML)
- Streaming and buffered output modes
- Column width calculation with Unicode support
- Plugin registry for extensibility (`RegisterFormatFunc`, `RegisterStreamingFormatter`, `RegisterValueFormatMode`)

**External deps:** `go-runewidthex`, `lox`, `tablewriter`, `go-iterator-helper` (stdlib only otherwise)
**Internal coupling:** Zero. `formatsql` imports it (unidirectional), not the reverse.

**Extraction readiness:** Excellent. Already uses plugin pattern that survives extraction unchanged.

**Why extract:**
- Sophisticated table formatting useful for any CLI/TUI tool
- Plugin architecture is non-trivial and reusable
- Go ecosystem lacks good options for streaming table output with width calculation

---

### 2. `metrics/` — Execution Metrics

**Location:** `internal/mycli/metrics/`
**Size:** ~550 LOC (code + tests)

**What it does:**
- Query lifecycle tracking (start, first row, last row, completion)
- TTFB calculation, row iteration timing, client overhead estimation
- Memory statistics (alloc, total, GC count)
- Server-side metric parsing

**External deps:** None (pure Go stdlib)
**Internal coupling:** Zero.

**Extraction readiness:** Excellent. Truly standalone data structures + helpers.

**Why extract:**
- Useful for any database client or profiling tool
- Zero dependencies makes it trivially extractable

---

### 3. `decoder/` — Spanner Value Decoding

**Location:** `internal/mycli/decoder/`
**Size:** ~710 LOC (code + tests)

**What it does:**
- Converts Spanner values to formatted strings
- Protocol buffer decoding via file descriptor sets
- Enum value resolution from proto definitions
- Type formatting helpers (simple and verbose)

**External deps:** `spanvalue`, `spantype`, `cloud.google.com/go/spanner`, `google.golang.org/protobuf`
**Internal coupling:** Zero.

**Extraction readiness:** Very good. Thin bridge between already-extracted packages.

**Why extract:**
- Useful for any Spanner tool needing proto/enum display
- Complements the already-extracted spanvalue/spantype ecosystem

---

### 4. `formatsql/` — SQL Export Formatter

**Location:** `internal/mycli/formatsql/`
**Size:** ~1,000 LOC (code + tests)

**What it does:**
- Generates INSERT/INSERT OR IGNORE/INSERT OR UPDATE statements from query results
- Table name extraction from queries (memefish AST)
- Batch INSERT with configurable row counts
- Reserved word quoting

**External deps:** `memefish` (SQL parsing), `format` package (formatter interface)
**Internal coupling:** Imports `format` (unidirectional, via registry pattern).

**Extraction readiness:** Very good. Clean plugin registration via `init()`.

**Why extract:**
- SQL export is reusable for data migration, backup, ETL tools
- Self-contained INSERT generation logic
- Depends on `format` interfaces — extract `format` first, or co-extract

---

## Tier 2: Good Candidates

### 5. `streamio/` — Stream Management

**Location:** `internal/mycli/streamio/`
**Size:** ~1,500 LOC (code + tests)

**What it does:**
- Centralizes stdin/stdout/stderr management with redirection
- TTY detection and terminal operations
- Tee file support (output to both stdout and file)
- TOCTOU race condition prevention for file safety
- Concurrent-safe stream operations

**External deps:** `golang.org/x/term` (stdlib otherwise)
**Internal coupling:** Zero.

**Extraction readiness:** Very good. Comprehensive test coverage (1,157 LOC tests).

**Why extract:**
- CLI tools often need tee + stream management
- TOCTOU safety logic is non-trivial and reusable

---

### 6. `filesafety/` — File Safety Validation

**Location:** `internal/mycli/filesafety/`
**Size:** ~300 LOC (code + tests)

**What it does:**
- Prevents reading from special files (devices, pipes, sockets)
- Configurable file size limits
- Safe read wrapper combining validation + `os.ReadFile`

**External deps:** None (pure Go stdlib)
**Internal coupling:** Zero.

**Extraction readiness:** Excellent. Minimal, focused, well-tested.

**Why extract:**
- Security-focused utility for any tool accepting file inputs
- Lightweight with zero dependencies

---

## Tier 3: Not Recommended

### `protostruct/`
- Too small (~2 KB) and too specific to Spanner proto handling
- Better kept internal or merged with `decoder`

---

## Extraction Roadmap

### Phase 1: Zero-Dependency Extractions (Low Effort, Low Risk)
1. **`metrics`** — standalone data structures, no deps
2. **`filesafety`** — minimal code, pure Go stdlib
3. **`streamio`** — only `golang.org/x/term` dependency

### Phase 2: Format Ecosystem (Medium Effort)
1. **`format`** first — foundation for plugin-based formatters
2. **`formatsql`** — depends on format interfaces
3. **`decoder`** — bridge between spanvalue/spantype

### Key Considerations
- All Tier 1 and 2 candidates have zero internal/mycli coupling
- Plugin registration pattern (`init()` + blank import) already in place for format/formatsql
- Established extraction pattern exists (spanvalue, spantype, etc.) for naming/structure reference
- **No backward compatibility required** — spanner-mycli does not expose these as a library

---

## Related Design Documents

- [Format Registry Refactoring](design-format-registry-refactoring.md) — ValueFormatMode, registry pattern details
