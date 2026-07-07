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

package bigquery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

// formatBigQueryValue renders a BigQuery result value as a display string.
//
// Tracked debt (#778 §1): values are rendered to strings, not typed cells, so a
// BIGQUERY result cannot carry column types downstream.
func formatBigQueryValue(v bq.Value, fieldType bq.FieldType) string {
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
		if fieldType == bq.BigNumericFieldType {
			return val.FloatString(38)
		}
		return val.FloatString(9)
	case bq.NullString:
		if !val.Valid {
			return "NULL"
		}
		return val.StringVal
	case bq.NullInt64:
		if !val.Valid {
			return "NULL"
		}
		return fmt.Sprintf("%d", val.Int64)
	case bq.NullFloat64:
		if !val.Valid {
			return "NULL"
		}
		return fmt.Sprintf("%g", val.Float64)
	case bq.NullBool:
		if !val.Valid {
			return "NULL"
		}
		if val.Bool {
			return "true"
		}
		return "false"
	case bq.NullTimestamp:
		if !val.Valid {
			return "NULL"
		}
		return val.Timestamp.Format(time.RFC3339Nano)
	case bq.NullDate:
		if !val.Valid {
			return "NULL"
		}
		return val.Date.String()
	case bq.NullTime:
		if !val.Valid {
			return "NULL"
		}
		return val.Time.String()
	case bq.NullDateTime:
		if !val.Valid {
			return "NULL"
		}
		return val.DateTime.String()
	case bq.NullJSON:
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
