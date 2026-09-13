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

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type receiptCompletion uint8

const (
	receiptCompleteNone receiptCompletion = iota
	receiptCompleteQuery
	receiptCompleteDML
	receiptCompleteBatch
)

// operationReceipt observes raw rows and terminal status for one user
// operation. A nil receipt is a no-op. Success can be finalized only once and
// only when every observed row completed and Finish/FinishDML/FinishBatch is
// called with a nil error. Formatter/pager errors and truncated iteration must
// not become a checkpointable success. Query and DML completion are distinct:
// a query fingerprint must not be reused as a DML fingerprint.
type operationReceipt struct {
	fp          *resultFingerprinter
	observed    int64
	failed      error
	finalized   bool
	complete    receiptCompletion
	fingerprint []byte
}

func (r *operationReceipt) ObserveMetadata(md *sppb.ResultSetMetadata) error {
	if r == nil {
		return nil
	}
	if r.finalized || r.failed != nil {
		if r.failed != nil {
			return r.failed
		}
		return fmt.Errorf("savepoint receipt already finalized")
	}
	if r.fp != nil {
		return nil
	}
	var fields []*sppb.StructType_Field
	if md != nil {
		fields = md.GetRowType().GetFields()
	}
	fp, err := newResultFingerprinter(fields)
	if err != nil {
		r.failed = err
		return err
	}
	r.fp = fp
	return nil
}

func (r *operationReceipt) ObserveRow(row *spanner.Row) error {
	if r == nil {
		return nil
	}
	if r.finalized || r.failed != nil {
		if r.failed != nil {
			return r.failed
		}
		return fmt.Errorf("savepoint receipt already finalized")
	}
	if r.fp == nil {
		fields, err := fieldsFromSpannerRow(row)
		if err != nil {
			r.failed = err
			return err
		}
		fp, err := newResultFingerprinter(fields)
		if err != nil {
			r.failed = err
			return err
		}
		r.fp = fp
	}
	if err := r.fp.ObserveRow(row); err != nil {
		r.failed = err
		return err
	}
	r.observed++
	return nil
}

func (r *operationReceipt) Finish(err error) ([]byte, error) {
	return r.finishWith(err, receiptCompleteQuery, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishQuery()
	})
}

func (r *operationReceipt) FinishDML(affected int64, err error) ([]byte, error) {
	return r.finishWith(err, receiptCompleteDML, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishDML(affected)
	})
}

func (r *operationReceipt) FinishBatch(counts []int64, err error) ([]byte, error) {
	return r.finishWith(err, receiptCompleteBatch, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishBatch(counts)
	})
}

func (r *operationReceipt) finishWith(err error, kind receiptCompletion, done func(*resultFingerprinter) ([]byte, error)) ([]byte, error) {
	if r == nil {
		return nil, err
	}
	if r.finalized {
		if err != nil {
			return nil, err
		}
		if r.failed != nil {
			return nil, r.failed
		}
		if r.complete != kind {
			return nil, fmt.Errorf("savepoint receipt completed as %s, not %s", r.complete, kind)
		}
		if r.fingerprint == nil {
			return nil, fmt.Errorf("savepoint receipt was not a successful observation")
		}
		return r.fingerprint, nil
	}
	r.finalized = true
	r.complete = kind
	if err != nil {
		r.failed = err
		return nil, err
	}
	if r.failed != nil {
		return nil, r.failed
	}
	if r.fp == nil {
		fp, fpErr := newResultFingerprinter(nil)
		if fpErr != nil {
			r.failed = fpErr
			return nil, fpErr
		}
		r.fp = fp
	}
	sum, fpErr := done(r.fp)
	if fpErr != nil {
		r.failed = fpErr
		return nil, fpErr
	}
	r.fingerprint = sum
	return sum, nil
}

func (c receiptCompletion) String() string {
	switch c {
	case receiptCompleteQuery:
		return "query"
	case receiptCompleteDML:
		return "dml"
	case receiptCompleteBatch:
		return "batch"
	default:
		return "none"
	}
}

func (r *operationReceipt) Succeeded() bool {
	return r != nil && r.finalized && r.failed == nil && r.fingerprint != nil
}

func fieldsFromSpannerRow(row *spanner.Row) ([]*sppb.StructType_Field, error) {
	if row == nil {
		return nil, fmt.Errorf("savepoint receipt observed a nil row")
	}
	fields := make([]*sppb.StructType_Field, row.Size())
	for i := range row.Size() {
		var gcv spanner.GenericColumnValue
		if err := row.Column(i, &gcv); err != nil {
			return nil, err
		}
		fields[i] = &sppb.StructType_Field{Name: row.ColumnName(i), Type: gcv.Type}
	}
	return fields, nil
}
