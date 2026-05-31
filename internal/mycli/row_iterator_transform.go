package mycli

import (
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanvalue/writer"
)

type rowIteratorSink[T any] struct {
	PrepareMetadata func(*sppb.ResultSetMetadata) error
	Write           func(T) error
	Finish          func(*writer.RowIteratorResult, int64) error
}

type rowIteratorRunConfig struct {
	metrics             *metrics.ExecutionMetrics
	transformErrorLabel string
	writeErrorLabel     string
	finishErrorLabel    string
	afterWriteRow       func() error
}

type rowIteratorRunOption func(*rowIteratorRunConfig)

func withRowIteratorMetrics(m *metrics.ExecutionMetrics) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.metrics = m
	}
}

func withRowIteratorErrorLabels(transformLabel, writeLabel, finishLabel string) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.transformErrorLabel = transformLabel
		c.writeErrorLabel = writeLabel
		c.finishErrorLabel = finishLabel
	}
}

func withRowIteratorAfterWriteRow(f func() error) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.afterWriteRow = f
	}
}

func runRowIteratorTransform[T any](
	iter *spanner.RowIterator,
	transform func(*spanner.Row) (T, error),
	sink rowIteratorSink[T],
	options ...rowIteratorRunOption,
) (*writer.RowIteratorResult, int64, error) {
	var cfg rowIteratorRunConfig
	for _, opt := range options {
		opt(&cfg)
	}

	var rowCount int64
	hooks := writer.RowIteratorHooks{
		PrepareMetadata: sink.PrepareMetadata,
		WriteRow: func(row *spanner.Row) error {
			now := time.Now()
			if cfg.metrics != nil && cfg.metrics.FirstRowTime == nil {
				cfg.metrics.FirstRowTime = &now
			}

			transformedRow, err := transform(row)
			if err != nil {
				return wrapRowIteratorError(cfg.transformErrorLabel, err)
			}
			if sink.Write != nil {
				if err := sink.Write(transformedRow); err != nil {
					return wrapRowIteratorRowError(cfg.writeErrorLabel, rowCount+1, err)
				}
			}

			rowCount++
			if cfg.metrics != nil {
				cfg.metrics.LastRowTime = &now
				cfg.metrics.RowCount = rowCount
			}

			if cfg.afterWriteRow != nil {
				if err := cfg.afterWriteRow(); err != nil {
					return err
				}
			}
			return nil
		},
		Finish: func(result *writer.RowIteratorResult) error {
			if sink.Finish == nil {
				return nil
			}
			if err := sink.Finish(result, rowCount); err != nil {
				return wrapRowIteratorError(cfg.finishErrorLabel, err)
			}
			return nil
		},
	}

	result, err := writer.RunRowIterator(iter, hooks)
	return result, rowCount, err
}

func wrapRowIteratorError(label string, err error) error {
	if err == nil || label == "" {
		return err
	}
	return fmt.Errorf("%s: %w", label, err)
}

func wrapRowIteratorRowError(label string, rowNumber int64, err error) error {
	if err == nil || label == "" {
		return err
	}
	return fmt.Errorf("%s %d: %w", label, rowNumber, err)
}
