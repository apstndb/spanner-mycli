// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodePartitionTokenRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 123456789, time.UTC)
	token, err := encodePartitionToken("projects/p/instances/i/databases/db", now, []byte("txid-bytes"), []byte("part-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, partitionTokenPrefix) {
		t.Fatalf("prefix = %q", token[:min(len(token), 20)])
	}
	decoded, err := decodePartitionToken(token, now)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Database != "projects/p/instances/i/databases/db" {
		t.Fatalf("database = %q", decoded.Database)
	}
	if string(decoded.TxID) != "txid-bytes" || string(decoded.Partition) != "part-bytes" {
		t.Fatalf("blobs tx=%q part=%q", decoded.TxID, decoded.Partition)
	}
	if !decoded.IssuedAt.Equal(now) || !decoded.NotAfter.Equal(now.Add(time.Hour)) {
		t.Fatalf("times issued=%s not_after=%s", decoded.IssuedAt, decoded.NotAfter)
	}
}

func TestEncodePartitionTokenUsesRFC3339NanoUTC(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 1, time.FixedZone("JST", 9*3600))
	token, err := encodePartitionToken("projects/p/instances/i/databases/db", now, []byte("t"), []byte("p"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, partitionTokenPrefix))
	if err != nil {
		t.Fatal(err)
	}
	var env partitionTokenJSON
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(env.IssuedAt.Format(time.RFC3339Nano), "Z") && env.IssuedAt.Location() != time.UTC {
		t.Fatalf("issued_at not UTC: %s loc=%v", env.IssuedAt, env.IssuedAt.Location())
	}
	if !env.IssuedAt.Equal(now.UTC()) {
		t.Fatalf("issued_at = %s, want %s", env.IssuedAt, now.UTC())
	}
}

func TestDecodePartitionTokenTimeRejects(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	db := "projects/p/instances/i/databases/db"
	valid, err := encodePartitionToken(db, now, []byte("t"), []byte("p"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		token string
		now   time.Time
		want  string
	}{
		{name: "legacy bare token", token: base64.StdEncoding.EncodeToString([]byte("only-pt")), now: now, want: "GetPartitionToken-only"},
		{name: "empty", token: "", now: now, want: "empty partition token"},
		{name: "missing version", token: "smycli-part/nopath", now: now, want: "missing version"},
		{name: "unsupported version", token: "smycli-part/2/" + base64.RawURLEncoding.EncodeToString([]byte(`{}`)), now: now, want: "unsupported partition token version"},
		{name: "expired at not_after", token: valid, now: now.Add(time.Hour), want: "exceeded client not_after"},
		{name: "expired after not_after", token: valid, now: now.Add(time.Hour + time.Second), want: "exceeded client not_after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodePartitionToken(tt.token, tt.now)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodePartitionTokenCraftedTimeRejects(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		env  partitionTokenJSON
		want string
	}{
		{
			name: "missing issued_at",
			env:  partitionTokenJSON{Database: "db", NotAfter: now.Add(time.Hour), TxID: b64("t"), Partition: b64("p")},
			want: "missing issued_at or not_after",
		},
		{
			name: "not_after equals issued_at",
			env:  partitionTokenJSON{Database: "db", IssuedAt: now, NotAfter: now, TxID: b64("t"), Partition: b64("p")},
			want: "not_after must be after issued_at",
		},
		{
			name: "span over one hour",
			env: partitionTokenJSON{
				Database:  "db",
				IssuedAt:  now.Add(-time.Hour - time.Second),
				NotAfter:  now.Add(time.Second),
				TxID:      b64("t"),
				Partition: b64("p"),
			},
			want: "advertised span exceeds 1h",
		},
		{
			name: "future issued_at",
			env: partitionTokenJSON{
				Database:  "db",
				IssuedAt:  now.Add(partitionTokenForwardClockSkew + time.Second),
				NotAfter:  now.Add(partitionTokenForwardClockSkew + time.Hour + time.Second),
				TxID:      b64("t"),
				Partition: b64("p"),
			},
			want: "issued_at is in the future",
		},
		{
			name: "not_after beyond hour plus skew uses span check first",
			env: partitionTokenJSON{
				Database:  "db",
				IssuedAt:  now,
				NotAfter:  now.Add(partitionTokenValidity + partitionTokenForwardClockSkew + time.Second),
				TxID:      b64("t"),
				Partition: b64("p"),
			},
			want: "advertised span exceeds 1h",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, err := encodePartitionTokenRaw(tt.env)
			if err != nil {
				t.Fatal(err)
			}
			_, err = decodePartitionToken(token, now)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodePartitionTokenAcceptsOneMinuteSkew(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	token, err := encodePartitionTokenRaw(partitionTokenJSON{
		Database:  "db",
		IssuedAt:  now.Add(partitionTokenForwardClockSkew),
		NotAfter:  now.Add(partitionTokenForwardClockSkew + time.Hour),
		TxID:      b64("t"),
		Partition: b64("p"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePartitionToken(token, now); err != nil {
		t.Fatalf("skew at bound should be accepted: %v", err)
	}
}

func TestPartitionTokenSizeCap(t *testing.T) {
	t.Parallel()
	huge := bytes.Repeat([]byte("A"), partitionTokenMaxEncodedBytes)
	if _, err := encodePartitionToken("db", time.Now().UTC(), huge, []byte("p")); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("encode oversized: %v", err)
	}
	oversized := partitionTokenPrefix + strings.Repeat("A", partitionTokenMaxEncodedBytes)
	if _, err := decodePartitionToken(oversized, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("decode oversized: %v", err)
	}
}

func TestEncodePartitionTokenRejectsEmptyInputs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	if _, err := encodePartitionToken("", now, []byte("t"), []byte("p")); err == nil || !strings.Contains(err.Error(), "database is empty") {
		t.Fatalf("empty database: %v", err)
	}
	if _, err := encodePartitionToken("db", now, nil, []byte("p")); err == nil || !strings.Contains(err.Error(), "empty native blob") {
		t.Fatalf("empty tx blob: %v", err)
	}
}

func TestPartitionTokenNowIsControllable(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	orig := partitionTokenNow
	partitionTokenNow = func() time.Time { return fixed }
	t.Cleanup(func() { partitionTokenNow = orig })
	token, err := encodePartitionToken("db", time.Time{}, []byte("t"), []byte("p"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePartitionToken(token, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.IssuedAt.Equal(fixed) {
		t.Fatalf("issued_at = %s, want %s", decoded.IssuedAt, fixed)
	}
}

func encodePartitionTokenRaw(env partitionTokenJSON) (string, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return partitionTokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
