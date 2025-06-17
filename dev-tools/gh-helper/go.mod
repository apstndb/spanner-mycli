module github.com/apstndb/spanner-mycli/dev-tools/gh-helper

go 1.22

require (
	github.com/apstndb/spanner-mycli/dev-tools/internal/shared v0.0.0
	github.com/spf13/cobra v1.8.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace github.com/apstndb/spanner-mycli/dev-tools/internal/shared => ../internal/shared
