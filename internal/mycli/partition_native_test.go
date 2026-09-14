// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"bytes"
	"encoding/gob"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
)

const (
	testPartitionDBA = "projects/p/instances/i/databases/a"
	testPartitionDBB = "projects/p/instances/i/databases/b"
)

func TestInspectNativePartitionRejects(t *testing.T) {
	t.Parallel()
	sid := testPartitionDBA + "/sessions/s1"
	tid := []byte("tid-1")
	sql := "SELECT * FROM Singers"
	goodTx, err := craftTestTxID(tid, sid, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	goodPart, err := craftTestQueryPartition([]byte("pt-1"), &sppb.ExecuteSqlRequest{
		Session:     sid,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: tid}},
		Sql:         sql,
	})
	if err != nil {
		t.Fatal(err)
	}

	otherSID := testPartitionDBB + "/sessions/s2"
	mixedTx, err := craftTestTxID([]byte("other-tid"), otherSID, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}

	emptyReq, err := craftTestQueryPartition([]byte("pt-1"), &sppb.ExecuteSqlRequest{})
	if err != nil {
		t.Fatal(err)
	}
	wrongDBPart, err := craftTestQueryPartition([]byte("pt-1"), &sppb.ExecuteSqlRequest{
		Session:     otherSID,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: tid}},
		Sql:         sql,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongDBTx, err := craftTestTxID(tid, otherSID, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	readPart, err := craftTestReadPartition([]byte("pt-1"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		tx      []byte
		part    []byte
		wantDB  string
		wantErr string
	}{
		{name: "empty blobs", tx: nil, part: nil, wantDB: testPartitionDBA, wantErr: "empty native blob"},
		{name: "truncated tx", tx: goodTx[:3], part: goodPart, wantDB: testPartitionDBA, wantErr: "malformed txid blob"},
		{name: "truncated partition", tx: goodTx, part: goodPart[:3], wantDB: testPartitionDBA, wantErr: "malformed partition blob"},
		{name: "mixed blobs", tx: mixedTx, part: goodPart, wantDB: testPartitionDBA, wantErr: "native session mismatch"},
		{name: "empty request", tx: goodTx, part: emptyReq, wantDB: testPartitionDBA, wantErr: "empty"},
		{name: "inner session other database", tx: wrongDBTx, part: wrongDBPart, wantDB: testPartitionDBA, wantErr: "is not under database"},
		{name: "read partition", tx: goodTx, part: readPart, wantDB: testPartitionDBA, wantErr: "read partitions are not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := inspectNativePartition(tt.tx, tt.part, tt.wantDB)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}

	emptyPT, err := craftTestQueryPartition(nil, &sppb.ExecuteSqlRequest{
		Session:     sid,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: tid}},
		Sql:         sql,
	})
	if err != nil {
		t.Fatal(err)
	}
	emptySQL, err := craftTestQueryPartition([]byte("pt-1"), &sppb.ExecuteSqlRequest{
		Session:     sid,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: tid}},
		Sql:         "   ",
	})
	if err != nil {
		t.Fatal(err)
	}
	txnMismatch, err := craftTestQueryPartition([]byte("pt-1"), &sppb.ExecuteSqlRequest{
		Session:     sid,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: []byte("other")}},
		Sql:         sql,
	})
	if err != nil {
		t.Fatal(err)
	}
	more := []struct {
		name    string
		tx      []byte
		part    []byte
		wantErr string
	}{
		{name: "empty partition token", tx: goodTx, part: emptyPT, wantErr: "empty partition token"},
		{name: "empty SQL", tx: goodTx, part: emptySQL, wantErr: "empty ExecuteSqlRequest SQL"},
		{name: "txn mismatch", tx: goodTx, part: txnMismatch, wantErr: "native transaction id mismatch"},
	}
	for _, tt := range more {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := inspectNativePartition(tt.tx, tt.part, testPartitionDBA)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}

	inspected, err := inspectNativePartition(goodTx, goodPart, testPartitionDBA)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.SQL != sql || inspected.Session != sid || !bytes.Equal(inspected.TID, tid) {
		t.Fatalf("inspected = %+v", inspected)
	}
}

// craftTestQueryPartition is test-only construction of the v1.95.0 Partition
// gob/proto layout. Production does not ship a second encoder.
func craftTestQueryPartition(pt []byte, req *sppb.ExecuteSqlRequest) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pt); err != nil {
		return nil, err
	}
	if err := enc.Encode(false); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(raw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func craftTestReadPartition(pt []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pt); err != nil {
		return nil, err
	}
	if err := enc.Encode(true); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(&sppb.ReadRequest{Table: "T"})
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(raw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func craftTestTxID(tid []byte, sid string, rts time.Time) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(tid); err != nil {
		return nil, err
	}
	if err := enc.Encode(sid); err != nil {
		return nil, err
	}
	if err := enc.Encode(rts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
