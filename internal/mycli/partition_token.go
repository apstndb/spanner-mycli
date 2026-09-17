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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Experimental RUN PARTITION envelope (issue #45).
//
// Encoded form: smycli-part/1/<base64url(JSON)>
// JSON fields only: database, issued_at, not_after, txid, partition.
//
// issued_at and not_after use Go encoding/json time.Time, which is RFC3339Nano
// in UTC. Production encode always writes UTC via time.Time.UTC before marshal.
// Decode accepts the same encoding/json time.Time rules.
//
// txid and partition are standard-base64 of the pinned SDK
// BatchReadOnlyTransactionID.MarshalBinary and Partition.MarshalBinary bytes.
// The prefix is the only format/version marker. Experimental status lives in
// help and docs, not in the JSON.
//
// Advertised validity is a fixed one hour. A one-minute forward-clock
// tolerance applies on consume. This is client-side validity only: not
// authentication, not remote cleanup, and not a Java/JDBC wire format.
const (
	partitionTokenPrefix           = "smycli-part/1/"
	partitionTokenMaxEncodedBytes  = 1 << 20 // 1 MiB encoded token, export and decode
	partitionTokenValidity         = time.Hour
	partitionTokenForwardClockSkew = time.Minute
)

// partitionTokenNow is the controllable clock for token issue and consume.
// Tests replace it; production uses time.Now.
var partitionTokenNow = time.Now

// partitionTokenJSON is the v1 envelope object. Do not add format/version
// fields; the prefix carries that.
type partitionTokenJSON struct {
	Database  string    `json:"database"`
	IssuedAt  time.Time `json:"issued_at"`
	NotAfter  time.Time `json:"not_after"`
	TxID      string    `json:"txid"`
	Partition string    `json:"partition"`
}

// decodedPartitionToken is the validated envelope after size, prefix, JSON,
// timestamp, and native-blob decode. Native consistency is a separate
// inspector step.
type decodedPartitionToken struct {
	Database  string
	IssuedAt  time.Time
	NotAfter  time.Time
	TxID      []byte
	Partition []byte
}

func encodePartitionToken(database string, now time.Time, txBlob, partBlob []byte) (string, error) {
	if now.IsZero() {
		now = partitionTokenNow()
	}
	now = now.UTC()
	if database == "" {
		return "", fmt.Errorf("partition token database is empty")
	}
	if len(txBlob) == 0 || len(partBlob) == 0 {
		return "", fmt.Errorf("empty native blob")
	}
	raw, err := json.Marshal(partitionTokenJSON{
		Database:  database,
		IssuedAt:  now,
		NotAfter:  now.Add(partitionTokenValidity),
		TxID:      base64.StdEncoding.EncodeToString(txBlob),
		Partition: base64.StdEncoding.EncodeToString(partBlob),
	})
	if err != nil {
		return "", err
	}
	token := partitionTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if len(token) > partitionTokenMaxEncodedBytes {
		return "", fmt.Errorf("partition token exceeds %d bytes before any decode", partitionTokenMaxEncodedBytes)
	}
	return token, nil
}

func decodePartitionToken(token string, now time.Time) (decodedPartitionToken, error) {
	var out decodedPartitionToken
	if now.IsZero() {
		now = partitionTokenNow()
	}
	now = now.UTC()
	if len(token) > partitionTokenMaxEncodedBytes {
		return out, fmt.Errorf("partition token exceeds %d bytes before any decode", partitionTokenMaxEncodedBytes)
	}
	if token == "" {
		return out, fmt.Errorf("empty partition token")
	}
	if !strings.HasPrefix(token, "smycli-part/") {
		return out, fmt.Errorf("unsupported partition token: missing smycli-part/ prefix (legacy GetPartitionToken-only values are not replayable)")
	}
	rest := strings.TrimPrefix(token, "smycli-part/")
	before, after, ok := strings.Cut(rest, "/")
	if !ok {
		return out, fmt.Errorf("malformed partition token: missing version")
	}
	ver := before
	if ver != "1" {
		return out, fmt.Errorf("unsupported partition token version %q", ver)
	}
	payload, err := base64.RawURLEncoding.DecodeString(after)
	if err != nil {
		return out, fmt.Errorf("malformed partition token payload: %w", err)
	}
	var env partitionTokenJSON
	if err := json.Unmarshal(payload, &env); err != nil {
		return out, fmt.Errorf("malformed partition token envelope: %w", err)
	}
	if err := validatePartitionTokenTimes(env, now); err != nil {
		return out, err
	}
	txBlob, err := base64.StdEncoding.DecodeString(env.TxID)
	if err != nil {
		return out, fmt.Errorf("malformed txid blob: %w", err)
	}
	partBlob, err := base64.StdEncoding.DecodeString(env.Partition)
	if err != nil {
		return out, fmt.Errorf("malformed partition blob: %w", err)
	}
	if len(txBlob) == 0 || len(partBlob) == 0 {
		return out, fmt.Errorf("empty native blob")
	}
	out = decodedPartitionToken{
		Database:  env.Database,
		IssuedAt:  env.IssuedAt,
		NotAfter:  env.NotAfter,
		TxID:      txBlob,
		Partition: partBlob,
	}
	return out, nil
}

func validatePartitionTokenTimes(env partitionTokenJSON, now time.Time) error {
	if env.IssuedAt.IsZero() || env.NotAfter.IsZero() {
		return fmt.Errorf("partition token missing issued_at or not_after")
	}
	if !env.NotAfter.After(env.IssuedAt) {
		return fmt.Errorf("partition token not_after must be after issued_at")
	}
	if env.NotAfter.Sub(env.IssuedAt) > partitionTokenValidity {
		return fmt.Errorf("partition token advertised span exceeds 1h")
	}
	if env.IssuedAt.After(now.Add(partitionTokenForwardClockSkew)) {
		return fmt.Errorf("partition token issued_at is in the future")
	}
	if env.NotAfter.After(now.Add(partitionTokenValidity + partitionTokenForwardClockSkew)) {
		return fmt.Errorf("partition token not_after exceeds 1h from now")
	}
	// Expired at or before now (not_after <= now).
	if !env.NotAfter.After(now) {
		return fmt.Errorf("partition token exceeded client not_after %s (this is not proof of backend cleanup)", env.NotAfter.UTC().Format(time.RFC3339))
	}
	return nil
}
