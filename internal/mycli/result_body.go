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

// resultBodyKind is the private discriminant for ResultBody. It distinguishes
// empty payloads (empty presentation table, zero-row typed data, zero-length
// prepared bytes) from the zero value, which means no body.
type resultBodyKind uint8

const (
	resultBodyNone resultBodyKind = iota
	resultBodyPresentation
	resultBodyTyped
	resultBodyPrepared
	resultBodyDelivered
)

// ResultBody is the closed payload of a Result. Shared metadata stays on Result.
// The zero value is no body and does not imply that output was already
// delivered. Constructors are the only way to set a kind; the discriminant is
// not settable. Feature packages construct presentation tables through
// PresentationBody.
type ResultBody struct {
	kind     resultBodyKind
	rows     []Row
	typed    *TypedRows
	prepared []byte
}

// PresentationBody is a display-text table, including a zero-row table whose
// headers still render. Passing a nil or empty slice is an explicit empty
// presentation table, not no-body.
func PresentationBody(rows []Row) ResultBody {
	return ResultBody{kind: resultBodyPresentation, rows: rows}
}

// TypedBody is a buffered result-set of raw *spanner.Row values. A TypedRows
// with metadata and zero rows is zero-row typed data, not no-body.
func TypedBody(typed *TypedRows) ResultBody {
	return ResultBody{kind: resultBodyTyped, typed: typed}
}

// PreparedBody is pre-rendered output that must be written as-is, including
// zero-length bytes. Empty prepared output bypasses table rendering rather than
// falling through to headers.
func PreparedBody(output []byte) ResultBody {
	return ResultBody{kind: resultBodyPrepared, prepared: output}
}

// DeliveredBody means the body was already written during execution. Appendices
// and summaries still print; no-body is a different kind.
func DeliveredBody() ResultBody {
	return ResultBody{kind: resultBodyDelivered}
}

// IsNone reports the zero value: no body, and not already delivered.
func (b ResultBody) IsNone() bool { return b.kind == resultBodyNone }

// AlreadyDelivered reports that execution already wrote the body.
func (b ResultBody) AlreadyDelivered() bool { return b.kind == resultBodyDelivered }

// PresentationRows returns the display-text rows when the body is a
// presentation table. ok is false for every other kind.
func (b ResultBody) PresentationRows() (rows []Row, ok bool) {
	if b.kind != resultBodyPresentation {
		return nil, false
	}
	return b.rows, true
}

// Typed returns the typed buffered payload when the body is typed rows.
func (b ResultBody) Typed() (typed *TypedRows, ok bool) {
	if b.kind != resultBodyTyped {
		return nil, false
	}
	return b.typed, true
}

// PreparedBytes returns the pre-rendered bytes when the body is prepared
// output. ok is true even when the slice is empty or nil.
func (b ResultBody) PreparedBytes() (output []byte, ok bool) {
	if b.kind != resultBodyPrepared {
		return nil, false
	}
	return b.prepared, true
}
