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
	"errors"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/google/go-cmp/cmp"
)

type recordingFormatter struct {
	inits     int
	finishes  int
	columns   []string
	preview   []Row
	rows      []Row
	initErr   error
	writeErr  error
	finishErr error
}

func (f *recordingFormatter) InitFormat(columnNames []string, _ format.FormatConfig, previewRows []Row) error {
	f.inits++
	f.columns = append([]string(nil), columnNames...)
	f.preview = append([]Row(nil), previewRows...)
	return f.initErr
}

func (f *recordingFormatter) WriteRow(row Row) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.rows = append(f.rows, append(Row(nil), row...))
	return nil
}

func (f *recordingFormatter) FinishFormat() error {
	f.finishes++
	return f.finishErr
}

func testResultMetadata() *sppb.ResultSetMetadata {
	return &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				{Name: "name", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			},
		},
	}
}

func TestTablePreviewProcessorPreviewThenPassThrough(t *testing.T) {
	t.Parallel()

	rec := &recordingFormatter{}
	p := NewTablePreviewProcessor(rec, 2)
	if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	rows := []Row{toRow("1", "Alice"), toRow("2", "Bob"), toRow("3", "Cara"), toRow("4", "Dan")}
	for i, row := range rows {
		if err := p.ProcessRow(row); err != nil {
			t.Fatalf("ProcessRow[%d]: %v", i, err)
		}
	}
	if err := p.Finish(QueryStats{}, int64(len(rows))); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if rec.inits != 1 || rec.finishes != 1 {
		t.Fatalf("lifecycle inits=%d finishes=%d, want 1/1", rec.inits, rec.finishes)
	}
	if diff := cmp.Diff([]string{"id", "name"}, rec.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if len(rec.preview) != 2 {
		t.Fatalf("preview len = %d, want 2", len(rec.preview))
	}
	if diff := cmp.Diff(rows, rec.rows); diff != "" {
		t.Errorf("written rows mismatch (-want +got):\n%s", diff)
	}
}

func TestTablePreviewProcessorHeadersOnlyStreamsImmediately(t *testing.T) {
	t.Parallel()

	rec := &recordingFormatter{}
	p := NewTablePreviewProcessor(rec, 0)
	if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	first := toRow("1", "Alice")
	if err := p.ProcessRow(first); err != nil {
		t.Fatalf("ProcessRow: %v", err)
	}
	if rec.inits != 1 {
		t.Fatalf("inits = %d, want 1 after first row", rec.inits)
	}
	if len(rec.preview) != 0 {
		t.Errorf("preview = %v, want empty for headers-only", rec.preview)
	}
	if diff := cmp.Diff([]Row{first}, rec.rows); diff != "" {
		t.Errorf("first row mismatch (-want +got):\n%s", diff)
	}

	second := toRow("2", "Bob")
	if err := p.ProcessRow(second); err != nil {
		t.Fatalf("ProcessRow after init: %v", err)
	}
	if err := p.Finish(QueryStats{}, 2); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if diff := cmp.Diff([]Row{first, second}, rec.rows); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}
}

func TestTablePreviewProcessorEmptyResultInitializesOnFinish(t *testing.T) {
	t.Parallel()

	rec := &recordingFormatter{}
	p := NewTablePreviewProcessor(rec, 50)
	if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Finish(QueryStats{}, 0); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if rec.inits != 1 || rec.finishes != 1 {
		t.Fatalf("empty result inits=%d finishes=%d, want 1/1", rec.inits, rec.finishes)
	}
	if len(rec.rows) != 0 {
		t.Errorf("rows = %v, want empty", rec.rows)
	}
}

func TestTablePreviewProcessorFewerRowsThanPreviewSize(t *testing.T) {
	t.Parallel()

	rec := &recordingFormatter{}
	p := NewTablePreviewProcessor(rec, 10)
	if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	row := toRow("1", "Alice")
	if err := p.ProcessRow(row); err != nil {
		t.Fatalf("ProcessRow: %v", err)
	}
	if rec.inits != 0 {
		t.Fatal("formatter initialized before preview was complete")
	}
	if err := p.Finish(QueryStats{}, 1); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if rec.inits != 1 {
		t.Fatalf("inits = %d, want 1 on Finish", rec.inits)
	}
	if diff := cmp.Diff([]Row{row}, rec.rows); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}
}

func TestTablePreviewProcessorErrorContracts(t *testing.T) {
	t.Parallel()

	md := testResultMetadata()
	row := toRow("1", "Alice")

	t.Run("InitFormat error", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{initErr: errors.New("init failed")}
		p := NewTablePreviewProcessor(rec, 1)
		if err := p.Init(md, format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(row); err == nil || !strings.Contains(err.Error(), "init failed") {
			t.Fatalf("ProcessRow error = %v, want init failed", err)
		}
	})

	t.Run("WriteRow error while flushing preview", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{writeErr: errors.New("write failed")}
		p := NewTablePreviewProcessor(rec, 1)
		if err := p.Init(md, format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(row); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("ProcessRow error = %v, want write failed", err)
		}
	})

	t.Run("WriteRow error after initialization", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{}
		p := NewTablePreviewProcessor(rec, 1)
		if err := p.Init(md, format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(row); err != nil {
			t.Fatalf("preview ProcessRow: %v", err)
		}
		rec.writeErr = errors.New("later write failed")
		if err := p.ProcessRow(toRow("2", "Bob")); err == nil || !strings.Contains(err.Error(), "later write failed") {
			t.Fatalf("ProcessRow error = %v, want later write failed", err)
		}
	})

	t.Run("FinishFormat error", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{finishErr: errors.New("finish failed")}
		p := NewTablePreviewProcessor(rec, 1)
		if err := p.Init(md, format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(row); err != nil {
			t.Fatalf("ProcessRow: %v", err)
		}
		if err := p.Finish(QueryStats{}, 1); err == nil || !strings.Contains(err.Error(), "finish failed") {
			t.Fatalf("Finish error = %v, want finish failed", err)
		}
	})

	t.Run("initializeFormatter is idempotent", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{}
		p := NewTablePreviewProcessor(rec, 1)
		if err := p.Init(md, format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(row); err != nil {
			t.Fatalf("ProcessRow: %v", err)
		}
		if err := p.initializeFormatter(); err != nil {
			t.Fatalf("second initializeFormatter: %v", err)
		}
		if rec.inits != 1 {
			t.Fatalf("inits = %d, want 1", rec.inits)
		}
		if len(rec.rows) != 1 {
			t.Fatalf("rows written twice: %v", rec.rows)
		}
	})
}

func TestStreamingProcessorLifecycle(t *testing.T) {
	t.Parallel()

	rec := &recordingFormatter{}
	p := NewStreamingProcessor(rec, nil, 80)
	if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	skipped := toRow("skip", "me")
	uninitialized := NewStreamingProcessor(rec, nil, 80)
	if err := uninitialized.ProcessRow(skipped); err != nil {
		t.Fatalf("uninitialized ProcessRow: %v", err)
	}
	if rec.rows != nil {
		t.Fatalf("uninitialized ProcessRow wrote %v", rec.rows)
	}

	rows := []Row{toRow("1", "Alice"), toRow("2", "Bob")}
	for i, row := range rows {
		if err := p.ProcessRow(row); err != nil {
			t.Fatalf("ProcessRow[%d]: %v", i, err)
		}
	}
	if err := p.Finish(QueryStats{}, 2); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if rec.inits != 1 || rec.finishes != 1 {
		t.Fatalf("lifecycle inits=%d finishes=%d, want 1/1", rec.inits, rec.finishes)
	}
	if diff := cmp.Diff([]string{"id", "name"}, rec.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(rows, rec.rows); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}
}

func TestStreamingProcessorErrorContracts(t *testing.T) {
	t.Parallel()

	t.Run("InitFormat error", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{initErr: errors.New("header failed")}
		p := NewStreamingProcessor(rec, nil, 0)
		if err := p.Init(testResultMetadata(), format.FormatConfig{}); err == nil || !strings.Contains(err.Error(), "header failed") {
			t.Fatalf("Init error = %v, want header failed", err)
		}
		if rec.finishes != 0 {
			t.Error("FinishFormat called after Init failure")
		}
	})

	t.Run("WriteRow error", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{writeErr: errors.New("row failed")}
		p := NewStreamingProcessor(rec, nil, 0)
		if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.ProcessRow(toRow("1", "Alice")); err == nil || !strings.Contains(err.Error(), "row failed") {
			t.Fatalf("ProcessRow error = %v, want row failed", err)
		}
	})

	t.Run("FinishFormat error", func(t *testing.T) {
		t.Parallel()
		rec := &recordingFormatter{finishErr: errors.New("close failed")}
		p := NewStreamingProcessor(rec, nil, 0)
		if err := p.Init(testResultMetadata(), format.FormatConfig{}); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := p.Finish(QueryStats{}, 0); err == nil || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("Finish error = %v, want close failed", err)
		}
	})
}
