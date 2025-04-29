//
// Copyright 2020 Google LLC
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
//

package main

import (
	_ "embed"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestBuildStatement(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339Nano, "2020-03-30T22:54:44.834017+09:00")
	if err != nil {
		t.Fatalf("unexpected time parse error: %v", err)
	}

	// valid tests
	for _, test := range []struct {
		desc          string
		input         string
		want          Statement
		skipLowerCase bool
	}{
		{
			desc:  "SELECT statement",
			input: "SELECT * FROM t1",
			want:  &SelectStatement{Query: "SELECT * FROM t1"},
		},
		{
			desc:  "SELECT statement in multiple lines",
			input: "SELECT\n*\nFROM t1",
			want:  &SelectStatement{Query: "SELECT\n*\nFROM t1"},
		},
		{
			desc:  "SELECT statement with comment",
			input: "SELECT 0x1/**/A",
			want:  &SelectStatement{Query: "SELECT 0x1/**/A"},
		},
		{
			desc:  "WITH statement",
			input: "WITH sub AS (SELECT 1) SELECT * FROM sub",
			want:  &SelectStatement{Query: "WITH sub AS (SELECT 1) SELECT * FROM sub"},
		},
		{
			// https://cloud.google.com/spanner/docs/query-syntax#statement-hints
			desc:  "SELECT statement with statement hint",
			input: "@{USE_ADDITIONAL_PARALLELISM=TRUE} SELECT * FROM t1",
			want:  &SelectStatement{Query: "@{USE_ADDITIONAL_PARALLELISM=TRUE} SELECT * FROM t1"},
		},
		{
			desc:  "Parenthesized SELECT statement",
			input: "(SELECT * FROM t1)",
			want:  &SelectStatement{Query: "(SELECT * FROM t1)"},
		},
		{
			desc:  "CREATE DATABASE statement",
			input: "CREATE DATABASE d1",
			want:  &CreateDatabaseStatement{CreateStatement: "CREATE DATABASE d1"},
		},
		{
			desc:  "DROP DATABASE statement",
			input: "DROP DATABASE d1",
			want:  &DropDatabaseStatement{DatabaseId: "d1"},
		},
		{
			desc:  "DROP DATABASE statement with escaped database name",
			input: "DROP DATABASE `TABLE`",
			want:  &DropDatabaseStatement{DatabaseId: "TABLE"},
		},
		{
			desc:  "ALTER DATABASE statement",
			input: "ALTER DATABASE d1 SET OPTIONS ( version_retention_period = '7d' )",
			want:  &DdlStatement{Ddl: `ALTER DATABASE d1 SET OPTIONS (version_retention_period = "7d")`},
		},
		{
			desc:  "CREATE TABLE statement",
			input: "CREATE TABLE t1 (id INT64 NOT NULL) PRIMARY KEY (id)",
			want:  &DdlStatement{Ddl: "CREATE TABLE t1 (\n  id INT64 NOT NULL\n) PRIMARY KEY (id)"},
		},
		{
			desc:  "RENAME TABLE statement",
			input: "RENAME TABLE t1 TO t2, t3 TO t4",
			want:  &DdlStatement{Ddl: "RENAME TABLE t1 TO t2, t3 TO t4"},
		},
		{
			desc:  "ALTER TABLE statement",
			input: "ALTER TABLE t1 ADD COLUMN name STRING(16) NOT NULL",
			want:  &DdlStatement{Ddl: "ALTER TABLE t1 ADD COLUMN name STRING(16) NOT NULL"},
		},
		{
			desc:  "DROP TABLE statement",
			input: "DROP TABLE t1",
			want:  &DdlStatement{Ddl: "DROP TABLE t1"},
		},
		{
			desc:  "CREATE INDEX statement",
			input: "CREATE INDEX idx_name ON t1 (name DESC)",
			want:  &DdlStatement{Ddl: "CREATE INDEX idx_name ON t1(name DESC)"},
		},
		{
			desc:  "DROP INDEX statement",
			input: "DROP INDEX idx_name",
			want:  &DdlStatement{Ddl: "DROP INDEX idx_name"},
		},
		{
			desc:  "TRUNCATE TABLE statement",
			input: "TRUNCATE TABLE t1",
			want:  &TruncateTableStatement{Table: "t1"},
		},
		{
			desc:  "CREATE VIEW statement",
			input: "CREATE VIEW t1view SQL SECURITY INVOKER AS SELECT t1.Id FROM t1",
			want:  &DdlStatement{Ddl: "CREATE VIEW t1view SQL SECURITY INVOKER AS SELECT t1.Id FROM t1"},
		},
		{
			desc:  "CREATE OR REPLACE VIEW statement",
			input: "CREATE OR REPLACE VIEW t1view SQL SECURITY INVOKER AS SELECT t1.Id FROM t1",
			want:  &DdlStatement{Ddl: "CREATE OR REPLACE VIEW t1view SQL SECURITY INVOKER AS SELECT t1.Id FROM t1"},
		},
		{
			desc:  "DROP VIEW statement",
			input: "DROP VIEW t1view",
			want:  &DdlStatement{Ddl: "DROP VIEW t1view"},
		},
		{
			desc:  "CREATE CHANGE STREAM FOR ALL statement",
			input: "CREATE CHANGE STREAM EverythingStream FOR ALL",
			want:  &DdlStatement{Ddl: "CREATE CHANGE STREAM EverythingStream FOR ALL"},
		},
		{
			desc:  "CREATE CHANGE STREAM FOR specific columns statement",
			input: "CREATE CHANGE STREAM NamesAndTitles FOR Singers(FirstName, LastName), Albums(Title)",
			want:  &DdlStatement{Ddl: "CREATE CHANGE STREAM NamesAndTitles FOR Singers(FirstName, LastName), Albums(Title)"},
		},
		{
			desc:  "ALTER CHANGE STREAM SET FOR statement",
			input: "ALTER CHANGE STREAM NamesAndAlbums SET FOR Singers(FirstName, LastName), Albums, Songs",
			want:  &DdlStatement{Ddl: "ALTER CHANGE STREAM NamesAndAlbums SET FOR Singers(FirstName, LastName), Albums, Songs"},
		},
		{
			desc:  "ALTER CHANGE STREAM SET OPTIONS statement",
			input: "ALTER CHANGE STREAM NamesAndAlbums SET OPTIONS( retention_period = '36h' )",
			want:  &DdlStatement{Ddl: `ALTER CHANGE STREAM NamesAndAlbums SET OPTIONS (retention_period = "36h")`},
		},
		{
			desc:  "ALTER CHANGE STREAM DROP FOR ALL statement",
			input: "ALTER CHANGE STREAM MyStream DROP FOR ALL",
			want:  &DdlStatement{Ddl: "ALTER CHANGE STREAM MyStream DROP FOR ALL"},
		},
		{
			desc:  "DROP CHANGE STREAM statement",
			input: "DROP CHANGE STREAM NamesAndAlbums",
			want:  &DdlStatement{Ddl: "DROP CHANGE STREAM NamesAndAlbums"},
		},
		{
			desc:  "GRANT statement",
			input: "GRANT SELECT ON TABLE employees TO ROLE hr_rep",
			want:  &DdlStatement{Ddl: "GRANT SELECT ON TABLE employees TO ROLE hr_rep"},
		},
		{
			desc:  "REVOKE statement",
			input: "REVOKE SELECT ON TABLE employees FROM ROLE hr_rep",
			want:  &DdlStatement{Ddl: "REVOKE SELECT ON TABLE employees FROM ROLE hr_rep"},
		},
		{
			desc:  "ALTER STATISTICS statement",
			input: "ALTER STATISTICS package SET OPTIONS (allow_gc = false)",
			want:  &DdlStatement{Ddl: "ALTER STATISTICS package SET OPTIONS (allow_gc = false)"},
		},
		{
			desc:  "ANALYZE statement",
			input: "ANALYZE",
			want:  &DdlStatement{Ddl: "ANALYZE"},
		},
		{
			desc:  "INSERT statement",
			input: "INSERT INTO t1 (id, name) VALUES (1, 'yuki')",
			want:  &DmlStatement{Dml: "INSERT INTO t1 (id, name) VALUES (1, 'yuki')"},
		},
		{
			desc:  "UPDATE statement",
			input: "UPDATE t1 SET name = hello WHERE id = 1",
			want:  &DmlStatement{Dml: "UPDATE t1 SET name = hello WHERE id = 1"},
		},
		{
			desc:  "DELETE statement",
			input: "DELETE FROM t1 WHERE id = 1",
			want:  &DmlStatement{Dml: "DELETE FROM t1 WHERE id = 1"},
		},
		{
			desc:  "PARTITIONED UPDATE statement",
			input: "PARTITIONED UPDATE t1 SET name = hello WHERE id > 1",
			want:  &PartitionedDmlStatement{Dml: "UPDATE t1 SET name = hello WHERE id > 1"},
		},
		{
			desc:  "PARTITIONED UPDATE statement with statement hint",
			input: "PARTITIONED @{PDML_MAX_PARALLELISM=100} UPDATE t1 SET name = hello WHERE id > 1",
			want:  &PartitionedDmlStatement{Dml: "@{PDML_MAX_PARALLELISM=100} UPDATE t1 SET name = hello WHERE id > 1"},
		},
		{
			desc:  "PARTITIONED DELETE statement",
			input: "PARTITIONED DELETE FROM t1 WHERE id > 1",
			want:  &PartitionedDmlStatement{Dml: "DELETE FROM t1 WHERE id > 1"},
		},
		{
			desc:  "PARTITIONED DELETE statement with statement hint",
			input: "PARTITIONED @{PDML_MAX_PARALLELISM=100} DELETE FROM t1 WHERE id > 1",
			want:  &PartitionedDmlStatement{Dml: "@{PDML_MAX_PARALLELISM=100} DELETE FROM t1 WHERE id > 1"},
		},
		{
			desc:  "EXPLAIN INSERT statement",
			input: "EXPLAIN INSERT INTO t1 (id, name) VALUES (1, 'yuki')",
			want:  &ExplainStatement{Explain: "INSERT INTO t1 (id, name) VALUES (1, 'yuki')", IsDML: true},
		},
		{
			desc:  "EXPLAIN UPDATE statement",
			input: "EXPLAIN UPDATE t1 SET name = hello WHERE id = 1",
			want:  &ExplainStatement{Explain: "UPDATE t1 SET name = hello WHERE id = 1", IsDML: true},
		},
		{
			desc:  "EXPLAIN DELETE statement",
			input: "EXPLAIN DELETE FROM t1 WHERE id = 1",
			want:  &ExplainStatement{Explain: "DELETE FROM t1 WHERE id = 1", IsDML: true},
		},
		{
			desc:  "DESCRIBE DELETE statement",
			input: "DESCRIBE DELETE FROM t1 WHERE id = 1",
			want:  &DescribeStatement{Statement: "DELETE FROM t1 WHERE id = 1", IsDML: true},
		},
		{
			desc:  "EXPLAIN ANALYZE INSERT statement",
			input: "EXPLAIN ANALYZE INSERT INTO t1 (id, name) VALUES (1, 'yuki')",
			want:  &ExplainAnalyzeDmlStatement{Dml: "INSERT INTO t1 (id, name) VALUES (1, 'yuki')"},
		},
		{
			desc:  "EXPLAIN ANALYZE UPDATE statement",
			input: "EXPLAIN ANALYZE UPDATE t1 SET name = hello WHERE id = 1",
			want:  &ExplainAnalyzeDmlStatement{Dml: "UPDATE t1 SET name = hello WHERE id = 1"},
		},
		{
			desc:  "EXPLAIN ANALYZE DELETE statement",
			input: "EXPLAIN ANALYZE DELETE FROM t1 WHERE id = 1",
			want:  &ExplainAnalyzeDmlStatement{Dml: "DELETE FROM t1 WHERE id = 1"},
		},
		{
			desc:  "BEGIN statement",
			input: "BEGIN",
			want:  &BeginStatement{},
		},
		{
			desc:  "BEGIN TRANSACTION statement",
			input: "BEGIN TRANSACTION",
			want:  &BeginStatement{},
		},
		{
			desc:  "BEGIN RW statement",
			input: "BEGIN RW",
			want:  &BeginRwStatement{},
		},
		{
			desc:  "BEGIN PRIORITY statement",
			input: "BEGIN PRIORITY MEDIUM",
			want: &BeginStatement{
				Priority: sppb.RequestOptions_PRIORITY_MEDIUM,
			},
		},
		{
			desc:  "BEGIN statement with SERIALIZABLE",
			input: "BEGIN ISOLATION LEVEL SERIALIZABLE",
			want: &BeginStatement{
				IsolationLevel: sppb.TransactionOptions_SERIALIZABLE,
			},
		},
		{
			desc:  "BEGIN statement with REPEATABLE READ",
			input: "BEGIN ISOLATION LEVEL REPEATABLE READ",
			want: &BeginStatement{
				IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
			},
		},
		{
			desc:  "BEGIN statement with REPEATABLE READ and PRIORITY",
			input: "BEGIN ISOLATION LEVEL REPEATABLE READ PRIORITY MEDIUM",
			want: &BeginStatement{
				IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
				Priority:       sppb.RequestOptions_PRIORITY_MEDIUM,
			},
		},
		{
			desc:  "BEGIN RW PRIORITY statement",
			input: "BEGIN RW PRIORITY LOW",
			want: &BeginRwStatement{
				Priority: sppb.RequestOptions_PRIORITY_LOW,
			},
		},
		{
			desc:  "BEGIN RW statement with SERIALIZABLE",
			input: "BEGIN RW ISOLATION LEVEL SERIALIZABLE",
			want: &BeginRwStatement{
				IsolationLevel: sppb.TransactionOptions_SERIALIZABLE,
			},
		},
		{
			desc:  "BEGIN RW statement with REPEATABLE READ",
			input: "BEGIN RW ISOLATION LEVEL REPEATABLE READ",
			want: &BeginRwStatement{
				IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
			},
		},
		{
			desc:  "BEGIN RW statement with REPEATABLE READ and PRIORITY",
			input: "BEGIN RW ISOLATION LEVEL REPEATABLE READ PRIORITY MEDIUM",
			want: &BeginRwStatement{
				IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
				Priority:       sppb.RequestOptions_PRIORITY_MEDIUM,
			},
		},
		{
			desc:  "BEGIN RO statement",
			input: "BEGIN RO",
			want:  &BeginRoStatement{TimestampBoundType: timestampBoundUnspecified},
		},
		{
			desc:  "BEGIN RO staleness statement",
			input: "BEGIN RO 10",
			want:  &BeginRoStatement{Staleness: time.Duration(10 * time.Second), TimestampBoundType: exactStaleness},
		},
		{
			desc:          "BEGIN RO read timestamp statement",
			input:         "BEGIN RO 2020-03-30T22:54:44.834017+09:00",
			want:          &BeginRoStatement{Timestamp: timestamp, TimestampBoundType: readTimestamp},
			skipLowerCase: true,
		},
		{
			desc:  "BEGIN RO PRIORITY statement",
			input: "BEGIN RO PRIORITY LOW",
			want:  &BeginRoStatement{TimestampBoundType: timestampBoundUnspecified, Priority: sppb.RequestOptions_PRIORITY_LOW},
		},
		{
			desc:  "BEGIN RO staleness with PRIORITY statement",
			input: "BEGIN RO 10 PRIORITY HIGH",
			want: &BeginRoStatement{
				Staleness:          time.Duration(10 * time.Second),
				TimestampBoundType: exactStaleness,
				Priority:           sppb.RequestOptions_PRIORITY_HIGH,
			},
		},
		{
			desc:  "SET TRANSACTION READ ONLY statement",
			input: "SET TRANSACTION READ ONLY",
			want:  &SetTransactionStatement{IsReadOnly: true},
		},
		{
			desc:  "SET TRANSACTION READ WRITE statement",
			input: "SET TRANSACTION READ WRITE",
			want:  &SetTransactionStatement{IsReadOnly: false},
		},
		{
			desc:  "COMMIT statement",
			input: "COMMIT",
			want:  &CommitStatement{},
		},
		{
			desc:  "COMMIT TRANSACTION statement",
			input: "COMMIT TRANSACTION",
			want:  &CommitStatement{},
		},
		{
			desc:  "ROLLBACK statement",
			input: "ROLLBACK",
			want:  &RollbackStatement{},
		},
		{
			desc:  "ROLLBACK TRANSACTION statement",
			input: "ROLLBACK TRANSACTION",
			want:  &RollbackStatement{},
		},
		{
			desc:  "CLOSE statement",
			input: "CLOSE",
			want:  &RollbackStatement{},
		},
		{
			desc:  "CLOSE TRANSACTION statement",
			input: "CLOSE TRANSACTION",
			want:  &RollbackStatement{},
		},
		{
			desc:  "EXIT statement",
			input: "EXIT",
			want:  &ExitStatement{},
		},
		{
			desc:  "USE statement",
			input: "USE database2",
			want:  &UseStatement{Database: "database2"},
		},
		{
			desc:  "USE statement with quoted identifier",
			input: "USE `my-database`",
			want:  &UseStatement{Database: "my-database"},
		},
		{
			desc:  "USE statement with role",
			input: "USE database2 ROLE role2",
			want:  &UseStatement{Database: "database2", Role: "role2"},
		},
		{
			desc:  "USE statement with quoted identifier",
			input: "USE `my-database` ROLE `my-role`",
			want:  &UseStatement{Database: "my-database", Role: "my-role"},
		},
		{
			desc:  "SHOW DATABASES statement",
			input: "SHOW DATABASES",
			want:  &ShowDatabasesStatement{},
		},
		{
			desc:  "SHOW CREATE TABLE statement",
			input: "SHOW CREATE TABLE t1",
			want:  &ShowCreateStatement{ObjectType: "TABLE", Name: "t1"},
		},
		{
			desc:  "SHOW CREATE TABLE statement with a named schema",
			input: "SHOW CREATE TABLE sch1.t1",
			want:  &ShowCreateStatement{ObjectType: "TABLE", Schema: "sch1", Name: "t1"},
		},
		{
			desc:  "SHOW CREATE TABLE statement with quoted identifier",
			input: "SHOW CREATE TABLE `TABLE`",
			want:  &ShowCreateStatement{ObjectType: "TABLE", Name: "TABLE"},
		},
		{
			desc:  "SHOW CREATE INDEX statement",
			input: "SHOW CREATE INDEX t1ByCol ",
			want:  &ShowCreateStatement{ObjectType: "INDEX", Name: "t1ByCol"},
		},
		{
			desc:  "SHOW CREATE INDEX statement with a named schema",
			input: "SHOW CREATE INDEX sch1.t1ByCol",
			want:  &ShowCreateStatement{ObjectType: "INDEX", Schema: "sch1", Name: "t1ByCol"},
		},
		{
			desc:  "SHOW TABLES statement",
			input: "SHOW TABLES",
			want:  &ShowTablesStatement{},
		},
		{
			desc:  "SHOW TABLES statement with schema",
			input: "SHOW TABLES sch1",
			want:  &ShowTablesStatement{Schema: "sch1"},
		},
		{
			desc:  "SHOW TABLES statement with quoted schema",
			input: "SHOW TABLES `sch1`",
			want:  &ShowTablesStatement{Schema: "sch1"},
		},
		{
			desc:  "SHOW INDEX statement",
			input: "SHOW INDEX FROM t1",
			want:  &ShowIndexStatement{Table: "t1"},
		},
		{
			desc:  "SHOW INDEX statement with a named schema",
			input: "SHOW INDEX FROM sch1.t1",
			want:  &ShowIndexStatement{Schema: "sch1", Table: "t1"},
		},
		{
			desc:  "SHOW INDEXES statement",
			input: "SHOW INDEXES FROM t1",
			want:  &ShowIndexStatement{Table: "t1"},
		},
		{
			desc:  "SHOW INDEX statement with quoted identifier",
			input: "SHOW INDEX FROM `TABLE`",
			want:  &ShowIndexStatement{Table: "TABLE"},
		},
		{
			desc:  "SHOW KEYS statement",
			input: "SHOW KEYS FROM t1",
			want:  &ShowIndexStatement{Table: "t1"},
		},
		{
			desc:  "SHOW COLUMNS statement",
			input: "SHOW COLUMNS FROM t1",
			want:  &ShowColumnsStatement{Table: "t1"},
		},
		{
			desc:  "SHOW COLUMNS statement with a named schema",
			input: "SHOW COLUMNS FROM sch1.t1",
			want:  &ShowColumnsStatement{Schema: "sch1", Table: "t1"},
		},
		{
			desc:  "SHOW COLUMNS statement with quoted identifier",
			input: "SHOW COLUMNS FROM `TABLE`",
			want:  &ShowColumnsStatement{Table: "TABLE"},
		},
		{
			desc:  "EXPLAIN SELECT statement",
			input: "EXPLAIN SELECT * FROM t1",
			want:  &ExplainStatement{Explain: "SELECT * FROM t1"},
		},
		{
			desc:  "EXPLAIN SELECT statement with statement hint",
			input: "EXPLAIN @{OPTIMIZER_VERSION=latest} SELECT * FROM t1",
			want:  &ExplainStatement{Explain: "@{OPTIMIZER_VERSION=latest} SELECT * FROM t1"},
		},
		{
			desc:  "EXPLAIN SELECT statement with WITH",
			input: "EXPLAIN WITH t1 AS (SELECT 1) SELECT * FROM t1",
			want:  &ExplainStatement{Explain: "WITH t1 AS (SELECT 1) SELECT * FROM t1"},
		},
		{
			desc:  "GRAPH statement",
			input: "GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id",
			want:  &SelectStatement{Query: "GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id"},
		},
		{
			desc:  "EXPLAIN GRAPH statement",
			input: "EXPLAIN GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id",
			want:  &ExplainStatement{Explain: "GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id"},
		},
		{
			desc:  "EXPLAIN ANALYZE GRAPH statement",
			input: "EXPLAIN ANALYZE GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id",
			want:  &ExplainAnalyzeStatement{Query: "GRAPH FinGraph MATCH (n) RETURN LABELS(n) AS label, n.id"},
		},
		{
			desc:  "DESCRIBE SELECT statement",
			input: "DESCRIBE SELECT * FROM t1",
			want:  &DescribeStatement{Statement: "SELECT * FROM t1"},
		},
		{
			desc:  "Stored system procedures",
			input: `CALL cancel_query("1234567890123456789")`,
			want:  &SelectStatement{Query: `CALL cancel_query("1234567890123456789")`},
		},
		{
			desc:  "EXPLAIN Stored system procedures",
			input: `EXPLAIN CALL cancel_query("1234567890123456789")`,
			want:  &ExplainStatement{Explain: `CALL cancel_query("1234567890123456789")`},
		},
		{
			desc:  "EXPLAIN ANALYZE Stored system procedures",
			input: `EXPLAIN ANALYZE CALL cancel_query("1234567890123456789")`,
			want:  &ExplainAnalyzeStatement{Query: `CALL cancel_query("1234567890123456789")`},
		},
		{
			desc:  "PARTITION statement",
			input: `PARTITION SELECT * FROM Singers`,
			want:  &PartitionStatement{SQL: `SELECT * FROM Singers`},
		},
		{
			desc:  "TRY PARTITIONED QUERY statement",
			input: `TRY PARTITIONED QUERY SELECT * FROM Singers`,
			want:  &TryPartitionedQueryStatement{SQL: `SELECT * FROM Singers`},
		},
		{
			desc:  "RUN PARTITIONED QUERY statement",
			input: `RUN PARTITIONED QUERY SELECT * FROM Singers`,
			want:  &RunPartitionedQueryStatement{SQL: `SELECT * FROM Singers`},
		},
		{
			desc:  "RUN PARTITION statement",
			input: `RUN PARTITION '123456789'`,
			want:  &RunPartitionStatement{Token: `123456789`},
		},
		{
			desc: "ADD SPLIT POINTS statement with EXPIRED AT",
			input: `
ADD SPLIT POINTS EXPIRED AT "2020-01-01T00:00:00Z"
TABLE Singers (42)
INDEX SingersByFirstLastName ("John", "Doe")
INDEX SingersByFirstLastName ("Mary", "Sue") TableKey (12)
`,
			// timestamp literal can't be lower
			skipLowerCase: true,
			want: &AddSplitPointsStatement{
				SplitPoints: []*databasepb.SplitPoints{
					{
						Table:      "Singers",
						ExpireTime: timestamppb.New(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)),
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("42")}}},
						},
					},
					{
						Index:      "SingersByFirstLastName",
						ExpireTime: timestamppb.New(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)),
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("John"), structpb.NewStringValue("Doe")}}},
						},
					},
					{
						Index:      "SingersByFirstLastName",
						ExpireTime: timestamppb.New(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)),
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("Mary"), structpb.NewStringValue("Sue")}}},
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("12")}}},
						},
					},
				},
			},
		},
		{
			desc: "ADD SPLIT POINTS statement without EXPIRED AT",
			input: `
ADD SPLIT POINTS
TABLE Singers (42)
`,
			want: &AddSplitPointsStatement{
				SplitPoints: []*databasepb.SplitPoints{
					{
						Table:      "Singers",
						ExpireTime: nil,
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("42")}}},
						},
					},
				},
			},
		},
		{
			desc: "DROP SPLIT POINTS statement",
			input: `
DROP SPLIT POINTS
TABLE Singers (42)
INDEX SingersByFirstLastName ("John", "Doe")
INDEX SingersByFirstLastName ("Mary", "Sue") TableKey (12)
`,
			want: &AddSplitPointsStatement{
				SplitPoints: []*databasepb.SplitPoints{
					{
						Table:      "Singers",
						ExpireTime: &timestamppb.Timestamp{},
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("42")}}},
						},
					},
					{
						Index:      "SingersByFirstLastName",
						ExpireTime: &timestamppb.Timestamp{},
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("John"), structpb.NewStringValue("Doe")}}},
						},
					},
					{
						Index:      "SingersByFirstLastName",
						ExpireTime: &timestamppb.Timestamp{},
						Keys: []*databasepb.SplitPoints_Key{
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("Mary"), structpb.NewStringValue("Sue")}}},
							{KeyParts: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("12")}}},
						},
					},
				},
			},
		},
		{
			desc:  "SHOW SPLIT POINTS statement",
			input: `SHOW SPLIT POINTS`,
			want:  &ShowSplitPointsStatement{},
		},
		{
			desc:  "SHOW LOCAL PROTO statement",
			input: `SHOW LOCAL PROTO`,
			want:  &ShowLocalProtoStatement{},
		},
		{
			desc:  "SHOW REMOTE PROTO statement",
			input: `SHOW REMOTE PROTO`,
			want:  &ShowRemoteProtoStatement{},
		},
		{
			desc:  "SYNC PROTO BUNDLE UPSERT statement",
			input: "SYNC PROTO BUNDLE UPSERT (examples.ProtoType, examples.`EscapedType`, `examples.EscapedPath`)",
			want:  &SyncProtoStatement{UpsertPaths: sliceOf("examples.ProtoType", "examples.EscapedType", `examples.EscapedPath`)},
		},
		{
			desc:  "SYNC PROTO BUNDLE DELETE statement",
			input: "SYNC PROTO BUNDLE DELETE (examples.ProtoType, examples.`EscapedType`, `examples.EscapedPath`)",
			want:  &SyncProtoStatement{DeletePaths: sliceOf("examples.ProtoType", "examples.EscapedType", `examples.EscapedPath`)},
		},
		{
			desc:  "SET statement",
			input: `SET OPTIMIZER_VERSION = "3"`,
			want:  &SetStatement{VarName: "OPTIMIZER_VERSION", Value: `"3"`},
		},
		{
			desc:  "SET += statement",
			input: `SET CLI_PROTO_DESCRIPTOR_FILE += "./message_descriptors.pb"`,
			want:  &SetAddStatement{VarName: "CLI_PROTO_DESCRIPTOR_FILE", Value: `"./message_descriptors.pb"`},
		},
		{
			desc:  "SHOW VARIABLE statement",
			input: `SHOW VARIABLE OPTIMIZER_VERSION`,
			want:  &ShowVariableStatement{VarName: "OPTIMIZER_VERSION"},
		},
		{
			desc:  "SHOW VARIABLES statement",
			input: `SHOW VARIABLES`,
			want:  &ShowVariablesStatement{},
		},
		{
			desc:  "SET PARAM type statement",
			input: `SET PARAM string_type STRING`,
			want:  &SetParamTypeStatement{Name: "string_type", Type: "STRING"},
		},
		{
			desc:  "SET PARAM value statement",
			input: `SET PARAM bytes_value = b"foo"`,
			want:  &SetParamValueStatement{Name: "bytes_value", Value: `b"foo"`},
		},
		{
			desc:  "SHOW PARAMS statement",
			input: `SHOW PARAMS`,
			want:  &ShowParamsStatement{},
		},
		{
			desc:  "SHOW DDLS statement",
			input: `SHOW DDLS`,
			want:  &ShowDdlsStatement{},
		},
		{
			desc:  "GEMINI statement",
			input: `GEMINI "Show all tables"`,
			want:  &GeminiStatement{Text: "Show all tables"},
		},
		{
			desc:  "START BATCH DDL statement",
			input: `START BATCH DDL`,
			want:  &StartBatchStatement{Mode: batchModeDDL},
		},
		{
			desc:  "START BATCH DML statement",
			input: `START BATCH DML`,
			want:  &StartBatchStatement{Mode: batchModeDML},
		},
		{
			desc:  "ABORT BATCH statement",
			input: `ABORT BATCH`,
			want:  &AbortBatchStatement{},
		},
		{
			desc:  "ABORT BATCH statement",
			input: `ABORT BATCH TRANSACTION`,
			want:  &AbortBatchStatement{},
		},
		{
			desc:  "RUN BATCH statement",
			input: `RUN BATCH`,
			want:  &RunBatchStatement{},
		},
		{
			desc:  "SHOW QUERY PROFILES statement",
			input: `SHOW QUERY PROFILES`,
			want:  &ShowQueryProfilesStatement{},
		},
		{
			desc:  "SHOW QUERY PROFILE statement",
			input: `SHOW QUERY PROFILE 123456789`,
			want: &ShowQueryProfileStatement{
				Fprint: 123456789,
			},
		},
		{
			desc:  "MUTATE INSERT statement",
			input: `MUTATE MutationTest INSERT STRUCT(1 AS PK)`,
			want: &MutateStatement{
				Table:     "MutationTest",
				Operation: "INSERT",
				Body:      "STRUCT(1 AS PK)",
			},
		},
		{
			desc:  "MUTATE UPDATE statement",
			input: `MUTATE MutationTest UPDATE STRUCT(1 AS PK)`,
			want: &MutateStatement{
				Table:     "MutationTest",
				Operation: "UPDATE",
				Body:      "STRUCT(1 AS PK)",
			},
		},
		{
			desc:  "MUTATE REPLACE statement",
			input: `MUTATE MutationTest REPLACE STRUCT(1 AS PK)`,
			want: &MutateStatement{
				Table:     "MutationTest",
				Operation: "REPLACE",
				Body:      "STRUCT(1 AS PK)",
			},
		},
		{
			desc:  "MUTATE INSERT_OR_UPDATE statement",
			input: `MUTATE MutationTest INSERT_OR_UPDATE STRUCT(1 AS PK)`,
			want: &MutateStatement{
				Table:     "MutationTest",
				Operation: "INSERT_OR_UPDATE",
				Body:      "STRUCT(1 AS PK)",
			},
		},
		{
			desc:  "MUTATE DELETE statement",
			input: `MUTATE MutationTest DELETE ALL`,
			want: &MutateStatement{
				Table:     "MutationTest",
				Operation: "DELETE",
				Body:      "ALL",
			},
		},
		{
			desc:  "CQL statement",
			input: `CQL SELECT id, active, username FROM users`,
			want:  &CQLStatement{CQL: "SELECT id, active, username FROM users"},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got, err := BuildStatement(test.input)
			if err != nil {
				t.Fatalf("BuildStatement(%q) got error: %v", test.input, err)
			}
			if diff := cmp.Diff(test.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("BuildStatement(%q) differ: %v", test.input, diff)
			}
		})

		if !test.skipLowerCase {
			input := strings.ToLower(test.input)
			t.Run("Lower "+test.desc, func(t *testing.T) {
				got, err := BuildStatement(input)
				if err != nil {
					t.Fatalf("BuildStatement(%q) got error: %v", input, err)
				}
				// check only type
				gotType := reflect.TypeOf(got)
				wantType := reflect.TypeOf(test.want)
				if gotType != wantType {
					t.Errorf("BuildStatement(%q) has invalid statement type: got = %q, but want = %q", input, gotType, wantType)
				}
			})
		}
	}
}

func TestBuildStatement_InvalidCase(t *testing.T) {
	// invalid tests
	for _, test := range []struct {
		input string
	}{
		{"FOO BAR"},
		{"SELEC T FROM t1"},
		{"BEGIN PRIORITY CRITICAL"},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := BuildStatement(test.input)
			if err == nil {
				t.Errorf("BuildStatement(%q) = %#v, but want error", test.input, got)
			}
		})
	}
}

func TestIsCreateDDL(t *testing.T) {
	for _, tt := range []struct {
		desc       string
		ddl        string
		objectType string
		schema     string
		table      string
		want       bool
	}{
		{
			desc:       "exact match",
			ddl:        "CREATE TABLE t1 (\n",
			objectType: "TABLE",
			table:      "t1",
			want:       true,
		},
		{
			desc:       "given table is prefix of DDL's table",
			ddl:        "CREATE TABLE t12 (\n",
			objectType: "TABLE",
			table:      "t1",
			want:       false,
		},
		{
			desc:       "DDL's table is prefix of given table",
			ddl:        "CREATE TABLE t1 (\n",
			objectType: "TABLE",
			table:      "t12",
			want:       false,
		},
		{
			desc:       "given table has reserved word",
			ddl:        "CREATE TABLE `create` (\n",
			objectType: "TABLE",
			table:      "create",
			want:       true,
		},
		{
			desc:       "given table is regular expression",
			ddl:        "CREATE TABLE t1 (\n",
			objectType: "TABLE",
			table:      `..`,
			want:       false,
		},
		{
			desc:       "given table is invalid regular expression",
			ddl:        "CREATE TABLE t1 (\n",
			objectType: "TABLE",
			table:      `[\]`,
			want:       false,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			if got := isCreateDDL(tt.ddl, tt.objectType, tt.schema, tt.table); got != tt.want {
				t.Errorf("isCreateDDL(%q, %q, %q) = %v, but want %v", tt.ddl, tt.objectType, tt.table, got, tt.want)
			}
		})
	}
}

func TestExtractSchemaAndTable(t *testing.T) {
	for _, tt := range []struct {
		desc   string
		input  string
		schema string
		table  string
	}{
		{
			desc:   "raw table",
			input:  "table",
			schema: "",
			table:  "table",
		},
		{
			desc:   "quoted table",
			input:  "`table`",
			schema: "",
			table:  "table",
		},
		{
			desc:   "FQN",
			input:  "schema.table",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN with spaces",
			input:  "schema . table",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN, both schema and table are quoted",
			input:  "`schema`.`table`",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN with spaces, both schema and table are quoted",
			input:  "`schema` . `table`",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN, only schema is quoted",
			input:  "`schema`.table",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN with spaces, only schema is quoted",
			input:  "`schema` . table",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN, only table is quoted",
			input:  "schema.`table`",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "FQN with spaces, only table is quoted",
			input:  "schema . `table`",
			schema: "schema",
			table:  "table",
		},
		{
			desc:   "whole quoted FQN",
			input:  "`schema.table`",
			schema: "schema",
			table:  "table",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			if schema, table := extractSchemaAndName(tt.input); schema != tt.schema || table != tt.table {
				t.Errorf("extractSchemaAndName(%q) = (%v, %v), but want (%v, %v)", tt.input, schema, table, tt.schema, tt.table)
			}
		})
	}
}

//go:embed testdata/protos/order_descriptors.pb
var orderDescriptorsContent []byte

var orderFds = lo.Must(decodeMessage[descriptorpb.FileDescriptorSet, *descriptorpb.FileDescriptorSet](orderDescriptorsContent))

func decodeMessage[T any, PT interface {
	*T
	proto.Message
}](b []byte) (PT, error) {
	var result T
	var pt PT = &result

	err := proto.Unmarshal(b, pt)
	return pt, err
}

func TestComposeProtoBundleDDLs(t *testing.T) {
	for _, tt := range []struct {
		desc        string
		fds         *descriptorpb.FileDescriptorSet
		upsertPaths []string
		deletePaths []string
		want        []string
	}{
		{
			desc:        "non-empty fds and empty input",
			fds:         orderFds,
			upsertPaths: nil,
			deletePaths: nil,
			want:        nil,
		},
		{
			desc:        "nil fds and empty input",
			fds:         nil,
			upsertPaths: nil,
			deletePaths: nil,
			want:        nil,
		},
		{
			desc: "empty fds and empty input",
			fds:  &descriptorpb.FileDescriptorSet{},
			want: nil,
		},
		{
			desc:        "UPSERT existing type",
			fds:         orderFds,
			upsertPaths: sliceOf("examples.shipping.Order"),
			want:        sliceOf("ALTER PROTO BUNDLE UPDATE (examples.shipping.`Order`)"),
		},
		{
			desc:        "UPSERT non-existing type",
			fds:         orderFds,
			upsertPaths: sliceOf("examples.UnknownType"),
			want:        sliceOf("ALTER PROTO BUNDLE INSERT (examples.UnknownType)"),
		},
		{
			desc:        "DELETE existing type",
			fds:         orderFds,
			deletePaths: sliceOf("examples.shipping.Order"),
			want:        sliceOf("ALTER PROTO BUNDLE DELETE (examples.shipping.`Order`)"),
		},
		{
			desc:        "DELETE non-existing type",
			fds:         orderFds,
			deletePaths: sliceOf("examples.UnknownType"),
			want:        nil,
		},
		// TODO: Support mixed SYNC PROTO BUNDLE in parseSyncProtoBundle
		{
			desc:        "Mixed UPSERT, DELETE",
			fds:         orderFds,
			upsertPaths: sliceOf("examples.shipping.Order", "examples.UnknownType"),
			deletePaths: sliceOf("examples.shipping.OrderHistory"),
			want:        sliceOf("ALTER PROTO BUNDLE INSERT (examples.UnknownType) UPDATE (examples.shipping.`Order`) DELETE (examples.shipping.OrderHistory)"),
		},
	} {
		got := composeProtoBundleDDLs(tt.fds, tt.upsertPaths, tt.deletePaths)
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("composeProtoBundleDDLs() mismatch (-want +got):\n%s", diff)
		}
	}
}
