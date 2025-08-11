package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/lox"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
)

type ShowCreateStatement struct {
	ObjectType string
	Schema     string
	Name       string
}

func (s *ShowCreateStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	ddlResponse, err := session.adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, stmt := range ddlResponse.Statements {
		if isCreateDDL(stmt, s.ObjectType, s.Schema, s.Name) {
			fqn := lox.IfOrEmpty(s.Schema != "", s.Schema+".") + s.Name
			rows = append(rows, toRow(fqn, stmt))
			break
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %q doesn't exist in schema %q", s.ObjectType, s.Name, s.Schema)
	}

	result := &Result{
		TableHeader:  toTableHeader("Name", "DDL"),
		Rows:         rows,
		AffectedRows: len(rows),
	}

	return result, nil
}

type ShowTablesStatement struct {
	Schema string
}

func (s *ShowTablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	alias := fmt.Sprintf("Tables_in_%s", session.systemVariables.Database)
	stmt := spanner.Statement{
		SQL:    fmt.Sprintf("SELECT t.TABLE_NAME AS `%s` FROM INFORMATION_SCHEMA.TABLES AS t WHERE t.TABLE_CATALOG = '' and t.TABLE_SCHEMA = @schema", alias),
		Params: map[string]any{"schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW TABLES", stmt, nil)
}

type ShowColumnsStatement struct {
	Schema string
	Table  string
}

func (s *ShowColumnsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
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

type ShowIndexStatement struct {
	Schema string
	Table  string
}

func (s *ShowIndexStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
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
  LOWER(I.TABLE_SCHEMA) = @table_schema AND LOWER(TABLE_NAME) = LOWER(@table_name)`,
		Params: map[string]any{"table_name": s.Table, "table_schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW INDEX", stmt, func() error {
		return fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	})
}

type ShowDdlsStatement struct{}

func (s *ShowDdlsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		KeepVariables: true,
		// intentionally empty column name to make TAB format valid DDL
		TableHeader: toTableHeader(""),
		Rows: sliceOf(toRow(stringsiter.Collect(xiter.Map(
			func(s string) string { return s + ";\n" },
			slices.Values(resp.GetStatements()))))),
	}, nil
}

// Helper functions for statements_schema.go

func executeInformationSchemaBasedStatement(ctx context.Context, session *Session, stmtName string, stmt spanner.Statement, emptyErrorF func() error) (*Result, error) {
	return executeInformationSchemaBasedStatementImpl(ctx, session, stmtName, stmt, false, emptyErrorF)
}

func executeInformationSchemaBasedStatementImpl(ctx context.Context, session *Session, stmtName string, stmt spanner.Statement, forceVerbose bool, emptyErrorF func() error) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, stmtName)
	}

	fc, err := formatConfigWithProto(session.systemVariables.ProtoDescriptor, session.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	iter, _ := session.RunQuery(ctx, stmt)

	rows, _, _, metadata, _, err := consumeRowIterCollect(iter, spannerRowToRow(fc))
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
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

func isCreateDDL(ddl string, objectType string, schema string, table string) bool {
	objectType = strings.ReplaceAll(objectType, " ", `\s+`)
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
