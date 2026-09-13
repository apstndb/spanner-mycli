// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	receipt             *operationReceipt
}

type rowIteratorRunOption func(*rowIteratorRunConfig)

func withRowIteratorMetrics(m *metrics.ExecutionMetrics) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.metrics = m
	}
}

func withRowIteratorReceipt(rec *operationReceipt) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.receipt = rec
	}
}

func withRowIteratorErrorLabels(transformLabel, writeLabel, finishLabel string) rowIteratorRunOption {
	return func(c *rowIteratorRunConfig) {
		c.transformErrorLabel = transformLabel
		c.writeErrorLabel = writeLabel
		c.finishErrorLabel = finishLabel
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
		PrepareMetadata: func(md *sppb.ResultSetMetadata) error {
			if err := cfg.receipt.ObserveMetadata(md); err != nil {
				return err
			}
			if sink.PrepareMetadata == nil {
				return nil
			}
			return sink.PrepareMetadata(md)
		},
		WriteRow: func(row *spanner.Row) error {
			if err := cfg.receipt.ObserveRow(row); err != nil {
				return err
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
				now := time.Now()
				if cfg.metrics.FirstRowTime == nil {
					firstRowTime := now
					cfg.metrics.FirstRowTime = &firstRowTime
				}
				if cfg.metrics.LastRowTime == nil {
					cfg.metrics.LastRowTime = new(time.Time)
				}
				*cfg.metrics.LastRowTime = now
				cfg.metrics.RowCount = rowCount
			}

			return nil
		},
		Finish: func(result *writer.RowIteratorResult) error {
			var finishErr error
			if sink.Finish != nil {
				finishErr = sink.Finish(result, rowCount)
				if finishErr != nil {
					finishErr = wrapRowIteratorError(cfg.finishErrorLabel, finishErr)
				}
			}
			if _, recErr := cfg.receipt.Finish(finishErr); recErr != nil && finishErr == nil {
				return recErr
			}
			return finishErr
		},
	}

	result, err := writer.RunRowIterator(iter, hooks)
	if err != nil {
		_, _ = cfg.receipt.Finish(err)
	}
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
