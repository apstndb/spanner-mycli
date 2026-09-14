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
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// DiscardUnknown stays false so unknown JSON fields are rejected atomically.
var directedReadJSONUnmarshal = protojson.UnmarshalOptions{}

var directedReadJSONMarshal = protojson.MarshalOptions{}

// forceNilDirectedReadOnCopiedClientConfig clears DirectedReadOptions on a
// ClientConfig that was copied from defaultClientConfig or EmbeddedClientConfig.
// Request-stamped QueryOptions are the only product routing source. Assigning
// nil (instead of leaving the copied field untouched) prevents an embedded DRO
// from remaining as an SDK client default after SET DIRECTED_READ clears.
// The embedder's original struct is not mutated.
func forceNilDirectedReadOnCopiedClientConfig(cfg *spanner.ClientConfig) {
	cfg.DirectedReadOptions = nil
}

func cloneDirectedRead(d *sppb.DirectedReadOptions) *sppb.DirectedReadOptions {
	if d == nil {
		return nil
	}
	return proto.CloneOf(d)
}

func parseDirectedReadJSON(jsonText string) (*sppb.DirectedReadOptions, error) {
	var opts sppb.DirectedReadOptions
	if err := directedReadJSONUnmarshal.Unmarshal([]byte(jsonText), &opts); err != nil {
		return nil, fmt.Errorf("invalid directed read protobuf JSON: %w", err)
	}
	return &opts, nil
}

func formatDirectedReadOption(d *sppb.DirectedReadOptions) string {
	if d == nil {
		return ""
	}
	if s, ok := losslessDirectedReadShorthand(d); ok {
		return s
	}
	b, err := directedReadJSONMarshal.Marshal(d)
	if err != nil {
		return ""
	}
	return string(b)
}

// losslessDirectedReadShorthand returns location or location:TYPE only when that
// text is nonempty, parses through the setter grammar, and proto.Equal-s the
// complete message. Shape alone is not enough: an empty replica SHOW would
// clear, and an unknown type number is not valid shorthand.
func losslessDirectedReadShorthand(d *sppb.DirectedReadOptions) (string, bool) {
	include := d.GetIncludeReplicas()
	if include == nil || !include.GetAutoFailoverDisabled() {
		return "", false
	}
	sels := include.GetReplicaSelections()
	if len(sels) != 1 || sels[0] == nil {
		return "", false
	}
	rs := sels[0]
	candidate := rs.GetLocation()
	if rs.GetType() != sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED {
		candidate = fmt.Sprintf("%s:%s", rs.GetLocation(), rs.GetType())
	}
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	parsed, err := parseDirectedReadOption(candidate)
	if err != nil || !proto.Equal(d, parsed) {
		return "", false
	}
	return candidate, true
}

func queryWithDirectedRead(ctx context.Context, txn *spanner.ReadOnlyTransaction, stmt spanner.Statement, dro *sppb.DirectedReadOptions) *spanner.RowIterator {
	return txn.QueryWithOptions(ctx, stmt, spanner.QueryOptions{DirectedReadOptions: dro})
}

func directedReadQueryOptions(dro *sppb.DirectedReadOptions) spanner.QueryOptions {
	return spanner.QueryOptions{DirectedReadOptions: dro}
}
