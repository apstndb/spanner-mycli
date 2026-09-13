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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
)

const savepointFingerprintDomain = "spanner-mycli/savepoint-fp/v1"

const (
	fpTagFields byte = iota + 1
	fpTagRow
	fpTagEOS
	fpTagAffected
	fpTagBatchCounts
)

var savepointProtoMarshal = proto.MarshalOptions{Deterministic: true, AllowPartial: true}

// resultFingerprinter hashes typed metadata and values before formatting.
// Plans, timestamps, profiles, and display text are excluded. Finish must
// be called after a successful end-of-stream; a truncated iterator is not a
// verified empty result.
type resultFingerprinter struct {
	h      hash.Hash
	fields []*sppb.StructType_Field
	rows   int64
	done   bool
}

func newResultFingerprinter(fields []*sppb.StructType_Field) (*resultFingerprinter, error) {
	f := &resultFingerprinter{h: sha256.New(), fields: cloneStructFields(fields)}
	f.h.Write([]byte(savepointFingerprintDomain))
	f.writeByte(fpTagFields)
	f.writeUint32(uint32(len(f.fields)))
	for _, field := range f.fields {
		name := ""
		var typ *sppb.Type
		if field != nil {
			name = field.GetName()
			typ = field.GetType()
		}
		f.writeBytes([]byte(name))
		if err := f.writeProto(typ); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (f *resultFingerprinter) ObserveRow(row *spanner.Row) error {
	if f == nil || f.done {
		return fmt.Errorf("savepoint fingerprint already finished")
	}
	if row == nil {
		return fmt.Errorf("savepoint fingerprint observed a nil row")
	}
	if row.Size() != len(f.fields) {
		return fmt.Errorf("savepoint fingerprint row width %d, want %d", row.Size(), len(f.fields))
	}
	values := make([]spanner.GenericColumnValue, row.Size())
	for i := range row.Size() {
		if err := row.Column(i, &values[i]); err != nil {
			return err
		}
	}
	return f.ObserveGCVs(values)
}

func (f *resultFingerprinter) ObserveGCVs(values []spanner.GenericColumnValue) error {
	if f == nil || f.done {
		return fmt.Errorf("savepoint fingerprint already finished")
	}
	if len(values) != len(f.fields) {
		return fmt.Errorf("savepoint fingerprint row width %d, want %d", len(values), len(f.fields))
	}
	f.writeByte(fpTagRow)
	f.writeUint32(uint32(len(values)))
	for i, v := range values {
		want := (*sppb.Type)(nil)
		if f.fields[i] != nil {
			want = f.fields[i].GetType()
		}
		if err := f.writeTypedValue(want, v); err != nil {
			return err
		}
	}
	f.rows++
	return nil
}

func (f *resultFingerprinter) FinishQuery() ([]byte, error) {
	return f.finish(func() {
		f.writeByte(fpTagEOS)
		f.writeUint64(uint64(f.rows))
	})
}

func (f *resultFingerprinter) FinishDML(affected int64) ([]byte, error) {
	return f.finish(func() {
		f.writeByte(fpTagEOS)
		f.writeUint64(uint64(f.rows))
		f.writeByte(fpTagAffected)
		f.writeUint64(uint64(affected))
	})
}

func (f *resultFingerprinter) FinishBatch(counts []int64) ([]byte, error) {
	return f.finish(func() {
		f.writeByte(fpTagEOS)
		f.writeByte(fpTagBatchCounts)
		f.writeUint32(uint32(len(counts)))
		for _, c := range counts {
			f.writeUint64(uint64(c))
		}
	})
}

func (f *resultFingerprinter) finish(extra func()) ([]byte, error) {
	if f == nil || f.done {
		return nil, fmt.Errorf("savepoint fingerprint already finished")
	}
	extra()
	f.done = true
	sum := f.h.Sum(nil)
	out := make([]byte, len(sum))
	copy(out, sum)
	return out, nil
}

func (f *resultFingerprinter) writeTypedValue(want *sppb.Type, v spanner.GenericColumnValue) error {
	if err := f.writeProto(v.Type); err != nil {
		return err
	}
	if err := f.writeProto(v.Value); err != nil {
		return err
	}
	if want != nil && v.Type != nil && !proto.Equal(want, v.Type) {
		return fmt.Errorf("savepoint fingerprint column type mismatch")
	}
	return nil
}

func (f *resultFingerprinter) writeProto(m proto.Message) error {
	if m == nil {
		f.writeBytes(nil)
		return nil
	}
	b, err := savepointProtoMarshal.Marshal(m)
	if err != nil {
		return err
	}
	f.writeBytes(b)
	return nil
}

func (f *resultFingerprinter) writeByte(b byte) {
	f.h.Write([]byte{b})
}

func (f *resultFingerprinter) writeUint32(n uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], n)
	f.h.Write(buf[:])
}

func (f *resultFingerprinter) writeUint64(n uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], n)
	f.h.Write(buf[:])
}

func (f *resultFingerprinter) writeBytes(b []byte) {
	f.writeUint32(uint32(len(b)))
	if len(b) > 0 {
		f.h.Write(b)
	}
}

func cloneStructFields(fields []*sppb.StructType_Field) []*sppb.StructType_Field {
	if fields == nil {
		return nil
	}
	out := make([]*sppb.StructType_Field, len(fields))
	for i, field := range fields {
		if field == nil {
			continue
		}
		out[i] = proto.Clone(field).(*sppb.StructType_Field)
	}
	return out
}
