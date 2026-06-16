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
