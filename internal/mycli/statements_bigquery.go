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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"google.golang.org/api/iterator"
)

// BigQuery related statements are registered in client_side_statement_def.go.

type BigQueryStatement struct {
	SQL string
}

func (s *BigQueryStatement) isDetachedCompatible() {}

func (s *BigQueryStatement) isConditionallyMutating() bool {
	return bigQueryStatementMutates(s.SQL)
}

// firstBigQueryKeyword returns the uppercased first whitespace-delimited token
// of a BigQuery statement, or "" if there is none.
func firstBigQueryKeyword(sql string) string {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

// bigQueryStatementMutates reports whether a BIGQUERY statement should be
// treated as mutating for the READONLY guard.
//
// The classifier is deliberately fail-closed: only BigQuery statements whose
// first keyword is a known read-only query verb are allowed under READONLY.
// Everything else, including unrecognized, empty, DML, DDL, and script-control
// statements, is treated as mutating and blocked before it can reach BigQuery.
func bigQueryStatementMutates(sql string) bool {
	switch firstBigQueryKeyword(sql) {
	case "SELECT", "WITH":
		return false
	default:
		return true
	}
}

func (s *BigQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	client, err := session.bigQueryClient(ctx)
	if err != nil {
		return nil, err
	}

	q := client.Query(s.SQL)
	if loc := session.systemVariables.Feature.BigQueryLocation; loc != "" {
		q.Location = loc
	}
	if max := session.systemVariables.Feature.BigQueryMaxBytesBilled; max != nil {
		q.MaxBytesBilled = *max
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}

	var rows []Row
	var values []bigquery.Value
	var headers []string
	for {
		err := it.Next(&values)
		if len(headers) == 0 {
			for _, field := range it.Schema {
				headers = append(headers, field.Name)
			}
		}
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		rowValues := make([]string, len(values))
		for i, v := range values {
			// Pass the schema field type so NUMERIC vs BIGNUMERIC scale can be
			// distinguished (both arrive as *big.Rat).
			var fieldType bigquery.FieldType
			if i < len(it.Schema) {
				fieldType = it.Schema[i].Type
			}
			rowValues[i] = formatBigQueryValue(v, fieldType)
		}
		rows = append(rows, toRow(rowValues...))
	}

	return &Result{
		TableHeader:  toTableHeader(headers),
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

func formatBigQueryValue(v bigquery.Value, fieldType bigquery.FieldType) string {
	if v == nil {
		return "NULL"
	}

	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case float32:
		return fmt.Sprintf("%g", val)
	case []byte:
		return base64.StdEncoding.EncodeToString(val)
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case civil.Date:
		return val.String()
	case civil.Time:
		return val.String()
	case civil.DateTime:
		return val.String()
	case *big.Rat:
		if val == nil {
			return "NULL"
		}
		// BigQuery returns both NUMERIC and BIGNUMERIC as *big.Rat. Select the
		// output scale from the schema field type so BIGNUMERIC keeps its full
		// precision (scale 38) instead of being truncated to NUMERIC scale (9).
		if fieldType == bigquery.BigNumericFieldType {
			return val.FloatString(38)
		}
		return val.FloatString(9)
	case bigquery.NullString:
		if !val.Valid {
			return "NULL"
		}
		return val.StringVal
	case bigquery.NullInt64:
		if !val.Valid {
			return "NULL"
		}
		return fmt.Sprintf("%d", val.Int64)
	case bigquery.NullFloat64:
		if !val.Valid {
			return "NULL"
		}
		return fmt.Sprintf("%g", val.Float64)
	case bigquery.NullBool:
		if !val.Valid {
			return "NULL"
		}
		if val.Bool {
			return "true"
		}
		return "false"
	case bigquery.NullTimestamp:
		if !val.Valid {
			return "NULL"
		}
		return val.Timestamp.Format(time.RFC3339Nano)
	case bigquery.NullDate:
		if !val.Valid {
			return "NULL"
		}
		return val.Date.String()
	case bigquery.NullTime:
		if !val.Valid {
			return "NULL"
		}
		return val.Time.String()
	case bigquery.NullDateTime:
		if !val.Valid {
			return "NULL"
		}
		return val.DateTime.String()
	case bigquery.NullJSON:
		if !val.Valid {
			return "NULL"
		}
		return string(val.JSONVal)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}
