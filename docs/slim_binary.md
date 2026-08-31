# Slim binary

`spanner-mycli-slim` is the release variant for users who do not need the
optional GEMINI/LLM, BigQuery, or Cassandra-interface CQL statement families.
It has the same core Cloud Spanner CLI behavior as `spanner-mycli`, but those
statement keywords and feature-specific flags and system variables are absent.

The variants are separate `main` packages, not build-tag variants. The full
root main imports `internal/mycli/feature/all`; the slim main imports only
`internal/mycli`. This keeps optional dependency graphs out of the slim binary
while ensuring `go test ./...`, vet, and lint compile both variants every time.

## Build and use

```sh
# Full binary, including all optional statement families.
CGO_ENABLED=0 go build -o spanner-mycli .

# Slim binary, excluding GEMINI/LLM, BIGQUERY, and CQL.
CGO_ENABLED=0 go build -o spanner-mycli-slim ./cmd/spanner-mycli-slim
./spanner-mycli-slim --version
```

GoReleaser publishes distinct `spanner-mycli` and `spanner-mycli-slim`
archives. Both builds embed the release version and `installFrom` metadata with
the same `main.version` and `main.installFrom` linker settings.

## Reproducible relative-size comparison

Use the same Go version, target platform, `CGO_ENABLED=0`, and stripping flags
for each binary. The following commands produce a relative size comparison.
They are not byte-identical to GoReleaser release archives (different
packaging, ldflags metadata, and archive format).

```sh
mkdir -p tmp/slim-size
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o tmp/slim-size/spanner-mycli .
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o tmp/slim-size/spanner-mycli-slim ./cmd/spanner-mycli-slim
wc -c tmp/slim-size/spanner-mycli tmp/slim-size/spanner-mycli-slim
```

The comparison is intentionally a build-time observation rather than a fixed
promise: linked size changes with the Go toolchain, target, and dependencies.
