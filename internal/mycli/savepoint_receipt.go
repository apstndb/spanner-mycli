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

// operationReceipt observes raw rows and terminal status for one user
// operation. A nil receipt is a no-op. Success can be finalized only once and
// only when every observed row completed and Finish is called with a nil
// error. Formatter/pager errors and truncated iteration must not become a
// checkpointable success.
type operationReceipt struct {
	fp          *resultFingerprinter
	observed    int64
	failed      error
	finalized   bool
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
	return r.finishWith(err, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishQuery()
	})
}

func (r *operationReceipt) FinishDML(affected int64, err error) ([]byte, error) {
	return r.finishWith(err, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishDML(affected)
	})
}

func (r *operationReceipt) FinishBatch(counts []int64, err error) ([]byte, error) {
	return r.finishWith(err, func(fp *resultFingerprinter) ([]byte, error) {
		return fp.FinishBatch(counts)
	})
}

func (r *operationReceipt) finishWith(err error, done func(*resultFingerprinter) ([]byte, error)) ([]byte, error) {
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
		if r.fingerprint == nil {
			return nil, fmt.Errorf("savepoint receipt was not a successful observation")
		}
		return r.fingerprint, nil
	}
	r.finalized = true
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
