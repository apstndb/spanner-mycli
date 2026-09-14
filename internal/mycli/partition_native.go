// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
)

// Decoder-only inspector of pinned cloud.google.com/go/spanner v1.95.0
// BatchReadOnlyTransactionID.MarshalBinary / Partition.MarshalBinary.
//
// The SDK does not expose Partition.qreq. This inspector reads the same gob
// field order and proto encoding as v1.95.0 batch.go MarshalBinary so native
// consistency can be checked before UnmarshalBinary / Execute. If that layout
// changes, this inspector must change with the pin. It is not a second
// production encoder; Execute still uses SDK UnmarshalBinary.

type inspectedNativePartition struct {
	SID      string
	TID      []byte
	RTS      time.Time
	PT       []byte
	SQL      string
	Session  string
	TxnID    []byte
	QueryReq *sppb.ExecuteSqlRequest
}

func inspectNativePartition(txBlob, partBlob []byte, wantDatabase string) (inspectedNativePartition, error) {
	var in inspectedNativePartition
	if len(txBlob) == 0 || len(partBlob) == 0 {
		return in, fmt.Errorf("empty native blob")
	}
	tdec := gob.NewDecoder(bytes.NewReader(txBlob))
	if err := tdec.Decode(&in.TID); err != nil {
		return in, fmt.Errorf("malformed txid blob: %w", err)
	}
	if err := tdec.Decode(&in.SID); err != nil {
		return in, fmt.Errorf("malformed txid blob: %w", err)
	}
	if err := tdec.Decode(&in.RTS); err != nil {
		return in, fmt.Errorf("malformed txid blob: %w", err)
	}
	pdec := gob.NewDecoder(bytes.NewReader(partBlob))
	if err := pdec.Decode(&in.PT); err != nil {
		return in, fmt.Errorf("malformed partition blob: %w", err)
	}
	var isRead bool
	if err := pdec.Decode(&isRead); err != nil {
		return in, fmt.Errorf("malformed partition blob: %w", err)
	}
	if isRead {
		return in, fmt.Errorf("read partitions are not supported; query-partition required")
	}
	var reqBytes []byte
	if err := pdec.Decode(&reqBytes); err != nil {
		return in, fmt.Errorf("malformed partition blob: %w", err)
	}
	req := &sppb.ExecuteSqlRequest{}
	if err := proto.Unmarshal(reqBytes, req); err != nil {
		return in, fmt.Errorf("malformed ExecuteSqlRequest: %w", err)
	}
	in.QueryReq = req
	in.SQL = req.GetSql()
	in.Session = req.GetSession()
	in.TxnID = req.GetTransaction().GetId()
	if err := validateInspectedNativePartition(in, wantDatabase); err != nil {
		return in, err
	}
	return in, nil
}

func validateInspectedNativePartition(in inspectedNativePartition, wantDatabase string) error {
	if len(in.PT) == 0 {
		return fmt.Errorf("empty partition token")
	}
	if in.SID == "" || len(in.TID) == 0 {
		return fmt.Errorf("empty transaction-id session or transaction")
	}
	if in.Session == "" || len(in.TxnID) == 0 {
		return fmt.Errorf("empty ExecuteSqlRequest session or transaction id")
	}
	if strings.TrimSpace(in.SQL) == "" {
		return fmt.Errorf("empty ExecuteSqlRequest SQL")
	}
	if in.Session != in.SID {
		return fmt.Errorf("native session mismatch between partition request and transaction-id blob")
	}
	if !bytes.Equal(in.TxnID, in.TID) {
		return fmt.Errorf("native transaction id mismatch between partition request and transaction-id blob")
	}
	prefix := wantDatabase + "/sessions/"
	if !strings.HasPrefix(in.Session, prefix) || len(in.Session) <= len(prefix) {
		return fmt.Errorf("partition request session %q is not under database %q", in.Session, wantDatabase)
	}
	return nil
}
