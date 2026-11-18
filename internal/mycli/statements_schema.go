package mycli

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanvalue"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
)

// showChangeStreamsGoogleSQL lists change streams for GoogleSQL-dialect databases.
// `ALL` is reserved and must be quoted; the view already describes the current
// database, so there is no catalog predicate.
const showChangeStreamsGoogleSQL = "SELECT CHANGE_STREAM_SCHEMA AS Schema, CHANGE_STREAM_NAME AS Name, `ALL` AS `All` FROM INFORMATION_SCHEMA.CHANGE_STREAMS ORDER BY CHANGE_STREAM_SCHEMA, CHANGE_STREAM_NAME"

// showChangeStreamsPostgreSQL lists change streams for PostgreSQL-dialect databases.
// information_schema.change_streams.all is YES/NO text; compare to YES so the
// result column is BOOL like GoogleSQL. Quote identifiers with double quotes.
const showChangeStreamsPostgreSQL = `SELECT change_stream_schema AS "Schema", change_stream_name AS "Name", ("all" = 'YES') AS "All" FROM information_schema.change_streams ORDER BY change_stream_schema, change_stream_name`

func showChangeStreamsSQL(dialect databasepb.DatabaseDialect) string {
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		return showChangeStreamsPostgreSQL
	}
	return showChangeStreamsGoogleSQL
}

type ShowCreateStatement struct {
	ObjectType string
	Schema     string
	Name       string
}

func (s *ShowCreateStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	ddlResponse, err := session.GetDatabaseDdlCached(ctx)
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, stmt := range ddlResponse.Statements {
		if isCreateDDL(stmt, s.ObjectType, s.Schema, s.Name) {
			fqn := lo.Ternary(s.Schema != "", s.Schema+".", lo.Empty[string]()) + s.Name
			rows = append(rows, toRow(fqn, stmt))
			break
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %q doesn't exist in schema %q", s.ObjectType, s.Name, s.Schema)
	}

	result := &Result{
		TableHeader:  toTableHeader("Name", "DDL"),
		Body:         PresentationBody(rows),
		AffectedRows: len(rows),
	}

	return result, nil
}

type ShowTablesStatement struct {
	Schema string
}

func (s *ShowTablesStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	alias := fmt.Sprintf("Tables_in_%s", session.systemVariables.Connection.Database)
	stmt := spanner.Statement{
		SQL:    fmt.Sprintf("SELECT t.TABLE_NAME AS %s FROM INFORMATION_SCHEMA.TABLES AS t WHERE t.TABLE_CATALOG = '' and t.TABLE_SCHEMA = @schema", spanvalue.QuoteIdentifier(session.systemVariables.Feature.DatabaseDialect, alias)),
		Params: map[string]any{"schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW TABLES", stmt, nil)
}

type ShowChangeStreamsStatement struct{}

func (s *ShowChangeStreamsStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt := spanner.Statement{SQL: showChangeStreamsSQL(session.systemVariables.Feature.DatabaseDialect)}
	return executeInformationSchemaBasedStatement(ctx, session, "SHOW CHANGE STREAMS", stmt, nil)
}

type ShowColumnsStatement struct {
	Schema string
	Table  string
}

func (s *ShowColumnsStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt := spanner.Statement{
		SQL: `SELECT
  C.COLUMN_NAME as Field,
  C.SPANNER_TYPE as Type,
  C.IS_NULLABLE as ` + "`NULL`" + `,
  I.INDEX_TYPE as Key,
  IC.COLUMN_ORDERING as Key_Order,
  CONCAT(CO.OPTION_NAME, "=", CO.OPTION_VALUE) as Options
FROM
  INFORMATION_SCHEMA.COLUMNS C
LEFT JOIN
  INFORMATION_SCHEMA.INDEX_COLUMNS IC USING(TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME)
LEFT JOIN
  INFORMATION_SCHEMA.INDEXES I USING(TABLE_SCHEMA, TABLE_NAME, INDEX_NAME)
LEFT JOIN
  INFORMATION_SCHEMA.COLUMN_OPTIONS CO USING(TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME)
WHERE
  LOWER(C.TABLE_SCHEMA) = LOWER(@table_schema) AND LOWER(C.TABLE_NAME) = LOWER(@table_name)
ORDER BY
  C.ORDINAL_POSITION ASC`,
		Params: map[string]any{"table_name": s.Table, "table_schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW COLUMNS", stmt, func() error {
		return fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	})
}

type ShowForeignKeysStatement struct {
	Schema string
	Table  string
}

// POSITION_IN_UNIQUE_CONSTRAINT maps a child column to its referenced column.
// Child and parent ordinals can differ, so both arrays are ordered by the
// child ordinal after pairing. DISTINCT avoids repeating columns if multiple
// foreign keys share a referenced backing index in the metadata.
const showForeignKeysSQL = `WITH fk_columns AS (
  SELECT DISTINCT
    rc.constraint_name,
    child.table_schema AS child_schema,
    child.table_name AS child_name,
    child.column_name AS child_column,
    child.ordinal_position AS child_position,
    parent.table_schema AS parent_schema,
    parent.table_name AS parent_name,
    parent.column_name AS parent_column,
    rc.delete_rule AS on_delete,
    (tc.enforced = 'YES') AS enforced,
    rc.spanner_state
  FROM information_schema.referential_constraints AS rc
  JOIN information_schema.key_column_usage AS child
    ON COALESCE(child.constraint_catalog, '') = COALESCE(rc.constraint_catalog, '')
    AND child.constraint_schema = rc.constraint_schema
    AND child.constraint_name = rc.constraint_name
  JOIN information_schema.key_column_usage AS parent
    ON COALESCE(parent.constraint_catalog, '') = COALESCE(rc.unique_constraint_catalog, '')
    AND parent.constraint_schema = rc.unique_constraint_schema
    AND parent.constraint_name = rc.unique_constraint_name
    AND parent.ordinal_position = child.position_in_unique_constraint
  JOIN information_schema.table_constraints AS tc
    ON COALESCE(tc.constraint_catalog, '') = COALESCE(rc.constraint_catalog, '')
    AND tc.constraint_schema = rc.constraint_schema
    AND tc.constraint_name = rc.constraint_name
    AND tc.table_schema = child.table_schema
    AND tc.table_name = child.table_name
    AND tc.constraint_type = 'FOREIGN KEY'
  WHERE (%s) OR (%s)
)
SELECT
  constraint_name,
  CASE WHEN child_schema = '' THEN child_name ELSE child_schema || '.' || child_name END AS child_table,
  ARRAY_AGG(child_column ORDER BY child_position) AS child_columns,
  CASE WHEN parent_schema = '' THEN parent_name ELSE parent_schema || '.' || parent_name END AS parent_table,
  ARRAY_AGG(parent_column ORDER BY child_position) AS parent_columns,
  on_delete,
  enforced,
  spanner_state
FROM fk_columns
GROUP BY constraint_name, child_schema, child_name, parent_schema, parent_name, on_delete, enforced, spanner_state
ORDER BY child_schema, child_name, constraint_name, parent_schema, parent_name`

func (s *ShowForeignKeysStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt := s.metadataStatement(session.systemVariables.Feature.DatabaseDialect)
	return executeInformationSchemaBasedStatement(ctx, session, "SHOW FOREIGN KEYS", stmt, nil)
}

func (s *ShowForeignKeysStatement) metadataStatement(dialect databasepb.DatabaseDialect) spanner.Statement {
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		schema := s.Schema
		if schema == "" {
			schema = "public"
		}
		return spanner.Statement{
			SQL: fmt.Sprintf(showForeignKeysSQL,
				"child.table_schema = $1 AND child.table_name = $2",
				"parent.table_schema = $1 AND parent.table_name = $2"),
			Params: map[string]any{"p1": schema, "p2": s.Table},
		}
	}
	return spanner.Statement{
		SQL: fmt.Sprintf(showForeignKeysSQL,
			"LOWER(child.table_schema) = LOWER(@schema) AND LOWER(child.table_name) = LOWER(@table)",
			"LOWER(parent.table_schema) = LOWER(@schema) AND LOWER(parent.table_name) = LOWER(@table)"),
		Params: map[string]any{"schema": s.Schema, "table": s.Table},
	}
}

type ShowIndexStatement struct {
	Schema string
	Table  string
}

func (s *ShowIndexStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt := spanner.Statement{
		SQL: `SELECT
  TABLE_NAME as Table,
  PARENT_TABLE_NAME as Parent_table,
  INDEX_NAME as Index_name,
  INDEX_TYPE as Index_type,
  IS_UNIQUE as Is_unique,
  IS_NULL_FILTERED as Is_null_filtered,
  INDEX_STATE as Index_state
FROM
  INFORMATION_SCHEMA.INDEXES I
WHERE
  LOWER(I.TABLE_SCHEMA) = LOWER(@table_schema) AND LOWER(TABLE_NAME) = LOWER(@table_name)`,
		Params: map[string]any{"table_name": s.Table, "table_schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW INDEX", stmt, func() error {
		return fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	})
}

type ShowDdlsStatement struct{}

func (s *ShowDdlsStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	resp, err := session.GetDatabaseDdlCached(ctx)
	if err != nil {
		return nil, err
	}

	return &Result{
		KeepVariables: true,
		// intentionally empty column name to make TAB format valid DDL
		TableHeader: toTableHeader(""),
		Body:        PresentationBody(sliceOf(toRow(strings.Join(slices.Collect(loi.Map(slices.Values(resp.GetStatements()), func(s string) string { return s + ";\n" })), "")))),
	}, nil
}

// Helper functions for statements_schema.go

func executeInformationSchemaBasedStatement(ctx context.Context, session *Session, stmtName string, stmt spanner.Statement, emptyErrorF func() error) (*Result, error) {
	return executeInformationSchemaBasedStatementImpl(ctx, session, stmtName, stmt, false, emptyErrorF)
}

func executeInformationSchemaBasedStatementImpl(ctx context.Context, session *Session, stmtName string, stmt spanner.Statement, forceVerbose bool, emptyErrorF func() error) (*Result, error) {
	if session.txn.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, stmtName)
	}

	fc, err := decoder.FormatConfigWithProto(session.systemVariables.Internal.ProtoDescriptor, session.systemVariables.Display.MultilineProtoText)
	if err != nil {
		return nil, err
	}
	fc, err = applyStringQuoteDisplay(fc, session.systemVariables.Display.StringQuoteMode, session.systemVariables.Display.CLIFormat)
	if err != nil {
		return nil, err
	}

	iter, _, err := session.txn.RunQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	rows, _, _, metadata, _, err := consumeRowIterCollect(iter, spannerRowToRow(fc, session.systemVariables.typeStyles, session.systemVariables.nullStyle))
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 && emptyErrorF != nil {
		return nil, emptyErrorF()
	}

	tableHeader := toTableHeader(metadata.GetRowType().GetFields())
	return &Result{
		// Pre-render only column names when forceVerbose is false
		TableHeader:  lo.Ternary[TableHeader](forceVerbose, tableHeader, toTableHeader(extractTableColumnNames(tableHeader))),
		ForceVerbose: forceVerbose,
		Body:         PresentationBody(rows),
		AffectedRows: len(rows),
	}, nil
}

func isCreateDDL(ddl string, objectType string, schema string, table string) bool {
	objectType = strings.ReplaceAll(objectType, " ", `\s+`)
	// Both user-supplied identifiers are interpolated into the regex below and
	// must be escaped; an unescaped schema would be interpreted as regex syntax.
	schema = regexp.QuoteMeta(schema)
	table = regexp.QuoteMeta(table)

	re := fmt.Sprintf("(?i)^CREATE (?:(?:NULL_FILTERED|UNIQUE) )?(?:OR REPLACE )?%s ", objectType)

	if schema != "" {
		re += fmt.Sprintf("(%[1]s|`%[1]s`)", schema)
		re += `\.`
	}

	re += fmt.Sprintf("(%[1]s|`%[1]s`)", table)
	re += `(?:\s+[^.]|$)`

	return regexp.MustCompile(re).MatchString(ddl)
}
