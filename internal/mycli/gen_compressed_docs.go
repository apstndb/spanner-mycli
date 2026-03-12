//go:build ignore

// gen_compressed_docs compresses official_docs/*.md to official_docs/*.zst
// using zstd for embedding.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	entries, err := os.ReadDir("official_docs")
	if err != nil {
		return fmt.Errorf("read official_docs: %w", err)
	}

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}
	defer enc.Close()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "README.md" {
			continue
		}

		mdPath := filepath.Join("official_docs", entry.Name())
		data, err := os.ReadFile(mdPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", mdPath, err)
		}

		compressed := enc.EncodeAll(data, nil)
		zstPath := filepath.Join("official_docs", strings.TrimSuffix(entry.Name(), ".md")+".zst")
		if err := os.WriteFile(zstPath, compressed, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", zstPath, err)
		}

		ratio := float64(len(data)) / float64(len(compressed))
		fmt.Printf("%s → %s (%d → %d bytes, %.1fx)\n", mdPath, zstPath, len(data), len(compressed), ratio)
	}

	return nil
}
