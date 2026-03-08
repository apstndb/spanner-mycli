package format

import (
	"fmt"
	"io"
	"sync"
)

// FormatFuncFactory creates a buffered FormatFunc for the given mode.
// The mode parameter allows a single factory to handle multiple related modes.
type FormatFuncFactory func(mode Mode) (FormatFunc, error)

// StreamingFormatterFactory creates a StreamingFormatter for the given mode.
// The mode parameter allows a single factory to handle multiple related modes.
type StreamingFormatterFactory func(mode Mode, out io.Writer, config FormatConfig) (StreamingFormatter, error)

var (
	registryMu           sync.RWMutex
	formatFuncRegistry   = map[Mode]FormatFuncFactory{}
	streamingFmtRegistry = map[Mode]StreamingFormatterFactory{}
)

// RegisterFormatFunc registers a FormatFuncFactory for the given modes.
// This allows external packages to add custom output formats (e.g., SQL export)
// without the format package depending on their implementation.
func RegisterFormatFunc(factory FormatFuncFactory, modes ...Mode) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, mode := range modes {
		formatFuncRegistry[mode] = factory
	}
}

// RegisterStreamingFormatter registers a StreamingFormatterFactory for the given modes.
// This allows external packages to add custom streaming output formats
// without the format package depending on their implementation.
func RegisterStreamingFormatter(factory StreamingFormatterFactory, modes ...Mode) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, mode := range modes {
		streamingFmtRegistry[mode] = factory
	}
}

// lookupFormatFunc looks up a registered FormatFuncFactory for the given mode.
func lookupFormatFunc(mode Mode) (FormatFuncFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := formatFuncRegistry[mode]
	return f, ok
}

// lookupStreamingFormatter looks up a registered StreamingFormatterFactory for the given mode.
func lookupStreamingFormatter(mode Mode) (StreamingFormatterFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := streamingFmtRegistry[mode]
	return f, ok
}

// errUnsupportedMode returns a formatted error for unsupported modes.
func errUnsupportedMode(kind string, mode Mode) error {
	return fmt.Errorf("unsupported %s mode: %s", kind, mode)
}
