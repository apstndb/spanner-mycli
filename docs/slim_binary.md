# Slim binary

`spanner-mycli-slim` is the release variant for users who do not need the
optional GEMINI/LLM, BigQuery, or Cassandra-interface CQL statement families.
It has the same core Cloud Spanner CLI behavior as `spanner-mycli`, including
experimental MCP (`--mcp`). Those optional statement keywords and
feature-specific flags and system variables are absent.

The variants are separate `main` packages, not build-tag variants. The full
root main imports `internal/mycli/feature/all`; the slim main imports only
`internal/mycli`. This keeps optional runtime dependency graphs out of the
slim binary while ensuring `go test ./...`, vet, and lint compile both
variants every time.

Slim is not a sandbox, a vulnerability-free binary, an AI-free binary, or a
promise of reduced `go.mod` / source / CI maintenance. The shared module still
declares optional-feature dependencies for the full graph, tests, and tools.
Core MCP and other remaining linked libraries stay present.

## Capability and config migration

If you currently pass GEMINI/CQL/BIGQUERY statements, `--vertexai-*` flags,
`CLI_VERTEXAI_*` / `CLI_GENAI_*` system variables, or the same keys in
`.spanner_mycli.toml`, they are **rejected** on slim (unknown statement, unknown
flag, or unknown configuration key), not silently ignored. Keep using the full
`spanner-mycli` binary for those capabilities. Container images from ko remain
the full variant only.

## Build and use

```sh
# Full binary, including all optional statement families.
CGO_ENABLED=0 go build -o spanner-mycli .

# Slim binary, excluding GEMINI/LLM, BIGQUERY, and CQL.
CGO_ENABLED=0 go build -o spanner-mycli-slim ./cmd/spanner-mycli-slim
./spanner-mycli-slim --version
```

GoReleaser publishes distinct `spanner-mycli` and `spanner-mycli-slim`
archives on the same OS/arch matrix, with Windows zip and identical
`main.version` / `main.installFrom` linker settings.

## Reproducible relative-size comparison

Use the same Go version, target platform, `CGO_ENABLED=0`, and stripping flags
for each binary. The following commands produce a relative size comparison.
They are not byte-identical to GoReleaser release archives (different
packaging, ldflags metadata, and archive format). Linked size is not the
adoption rationale and is not a fixed promise: it changes with the Go
toolchain, target, and dependencies.

```sh
mkdir -p tmp/slim-size
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o tmp/slim-size/spanner-mycli .
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o tmp/slim-size/spanner-mycli-slim ./cmd/spanner-mycli-slim
wc -c tmp/slim-size/spanner-mycli tmp/slim-size/spanner-mycli-slim
```
