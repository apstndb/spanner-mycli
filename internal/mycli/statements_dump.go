package mycli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/spanner"
	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DumpDatabaseStatement represents DUMP DATABASE statement
// It exports both DDL and data for all tables in the database
type DumpDatabaseStatement struct{}

func (s *DumpDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeDatabase, nil)
}

// DumpSchemaStatement represents DUMP SCHEMA statement
// It exports only DDL statements without any data
type DumpSchemaStatement struct{}

func (s *DumpSchemaStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeSchema, nil)
}

// DumpTablesStatement represents DUMP TABLES statement
// It exports data only for specified tables (no DDL)
type DumpTablesStatement struct {
	Tables []tableID
}

func (s *DumpTablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeTables, s.Tables)
}

// dumpMode represents the type of dump operation
type dumpMode int

const (
	dumpModeDatabase dumpMode = iota // Export DDL + all tables
	dumpModeSchema                   // Export DDL only
	dumpModeTables                   // Export specific tables only
)

func (m dumpMode) shouldExportData() bool { return m == dumpModeDatabase || m == dumpModeTables }

// executeDump is the main entry point for all dump operations.
// It decides between streaming and buffered mode based on the output stream and settings.
type dumpTablePlan struct {
	ID      tableID
	Columns []string
}

type dumpPlan struct {
	Data []dumpDataPlan
	DDL  []byte
}

// A data unit is either one ordinary table or one pre-encoded cyclic group.
// Empty marks a cyclic table already scanned in the planning snapshot; its
// legacy comment is emitted without querying that table a second time.
type dumpDataPlan struct {
	Table  dumpTablePlan
	Cyclic *dumpCyclicData
	Empty  bool
}

func executeDump(ctx context.Context, session *Session, mode dumpMode, specificTables []tableID) (*Result, error) {
	if session.adminClient == nil {
		return nil, fmt.Errorf("admin client is not initialized")
	}
	// TODO: Add proper PostgreSQL support. Currently the SQL export format depends on spanvalue.LiteralFormatConfig
	// which generates Google SQL literals, not PostgreSQL-compatible ones.
	if session.systemVariables.Feature.DatabaseDialect == dbadminpb.DatabaseDialect_POSTGRESQL {
		return nil, fmt.Errorf("DUMP statements are not yet supported for PostgreSQL dialect databases")
	}
	if mode == dumpModeSchema {
		plan, err := prepareDumpSchema(ctx, session)
		if err != nil {
			return nil, err
		}
		return writeDumpPlan(ctx, session, mode, plan, nil)
	}

	var result *Result
	err := session.txn.withReadOnlyTransactionOrStart(ctx, func(txn *spanner.ReadOnlyTransaction) error {
		plan, err := prepareDumpWithTxn(ctx, session, mode, specificTables, txn)
		if err != nil {
			return err
		}
		result, err = writeDumpPlan(ctx, session, mode, plan, txn)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func writeDumpPlan(ctx context.Context, session *Session, mode dumpMode, plan *dumpPlan, txn *spanner.ReadOnlyTransaction) (*Result, error) {
	outStream := session.outputWriter()
	if outStream != nil && outStream != io.Discard {
		return executeDumpStreamingWithTxn(ctx, session, mode, plan, outStream, txn)
	}
	return executeDumpBufferedWithTxn(ctx, session, mode, plan, txn)
}

// buildSelectQueryWithColumns creates a SELECT query with explicit column list.
// Identifiers are quoted via spanvalue's dialect-aware helpers so reserved words
// and qualified table names are rendered correctly for the current database.
func buildSelectQueryWithColumns(dialect dbadminpb.DatabaseDialect, columns []string, id tableID) string {
	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		quotedColumns[i] = spanvalue.QuoteIdentifier(dialect, col)
	}
	return fmt.Sprintf("SELECT %s FROM %s",
		strings.Join(quotedColumns, ", "),
		quoteTableID(dialect, id),
	)
}

// getWritableColumnsWithTxn queries INFORMATION_SCHEMA to get only columns that can accept INSERT values.
// It uses the provided transaction to ensure consistency with other queries.
// It excludes generated columns and other non-writable column types.
// Returns column names in their original form, ordered by ORDINAL_POSITION.
// NOTE: INFORMATION_SCHEMA queries cannot be used in read-write transactions.
func getWritableColumnsWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction, id tableID) ([]string, error) {
	query := `
		SELECT COLUMN_NAME
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = @schema
		  AND TABLE_NAME = @table
		  AND IS_GENERATED = 'NEVER'
		ORDER BY ORDINAL_POSITION`

	stmt := spanner.Statement{
		SQL: query,
		Params: map[string]interface{}{
			"schema": id.Schema,
			"table":  id.Name,
		},
	}

	var columns []string
	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	err := iter.Do(func(r *spanner.Row) error {
		var columnName string
		if err := r.Column(0, &columnName); err != nil {
			return err
		}
		columns = append(columns, columnName)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query writable columns for %s: %w", id.FQN(), err)
	}

	// Return empty slice for tables with no writable columns (e.g., all generated columns)
	// Callers should handle this case gracefully by skipping data export
	return columns, nil
}

func wrapDumpGetDdlError(err error) error {
	if status.Code(err) == codes.PermissionDenied {
		return fmt.Errorf("dump requires spanner.databases.getDdl permission: %w", err)
	}
	return fmt.Errorf("dump GetDatabaseDdl: %w", err)
}

func prepareDumpSchema(ctx context.Context, session *Session) (*dumpPlan, error) {
	ddlResult, err := exportDDL(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("export DDL: %w", err)
	}
	return &dumpPlan{DDL: ddlResult.RenderedOutput}, nil
}

func prepareDumpWithTxn(ctx context.Context, session *Session, mode dumpMode, specificTables []tableID, txn *spanner.ReadOnlyTransaction) (*dumpPlan, error) {
	plan := &dumpPlan{}
	var freshDDL []string
	var freshProto []byte
	haveFresh := false
	fetchFresh := func() ([]string, error) {
		if haveFresh {
			return freshDDL, nil
		}
		resp, err := session.GetDatabaseDdlFresh(ctx)
		if err != nil {
			return nil, wrapDumpGetDdlError(err)
		}
		freshDDL = resp.GetStatements()
		freshProto = resp.GetProtoDescriptors()
		haveFresh = true
		return freshDDL, nil
	}

	resolver := NewDependencyResolver()
	if err := resolver.BuildDependencyGraphWithTxn(ctx, txn); err != nil {
		return nil, err
	}
	if session.dumpReadTxnProbe != nil {
		session.dumpReadTxnProbe("catalog", txn)
	}
	var selected []tableID
	if specificTables != nil {
		for _, id := range specificTables {
			if err := resolver.lookupExplicit(id); err != nil {
				return nil, err
			}
		}
		selected = specificTables
	} else {
		for id := range resolver.tables {
			selected = append(selected, id)
		}
	}
	if mode == dumpModeDatabase {
		stmts, err := fetchFresh()
		if err != nil {
			return nil, err
		}
		plan.DDL, err = renderDumpDDL(stmts, freshProto)
		if err != nil {
			return nil, err
		}
		if err := resolver.applyInterleaveParents(stmts, selected); err != nil {
			return nil, err
		}
	} else if mode.shouldExportData() && resolver.selectedNeedsInterleaveDDL(selected) {
		stmts, err := fetchFresh()
		if err != nil {
			return nil, err
		}
		if err := resolver.applyInterleaveParents(stmts, selected); err != nil {
			return nil, err
		}
	}
	if !mode.shouldExportData() {
		return plan, nil
	}
	if session.systemVariables.Display.DumpCyclicMode == enums.DumpCyclicModeMutate {
		var err error
		plan.Data, err = prepareDumpMutationUnits(ctx, session, txn, resolver, selected)
		return plan, err
	}
	if err := rejectPopulatedCyclicDumpSCCs(ctx, session, txn, resolver, selected); err != nil {
		return nil, err
	}
	order, err := resolver.GetOrderForTables(selected)
	if err != nil {
		return nil, err
	}
	for _, id := range order {
		columns, err := getWritableColumnsWithTxn(ctx, txn, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get writable columns for table %s: %w", id.FQN(), err)
		}
		plan.Data = append(plan.Data, dumpDataPlan{Table: dumpTablePlan{ID: id, Columns: columns}})
	}
	return plan, nil
}

func executeDumpBufferedWithTxn(ctx context.Context, session *Session, mode dumpMode, plan *dumpPlan, txn *spanner.ReadOnlyTransaction) (*Result, error) {
	var out bytes.Buffer
	affectedRows, err := writeDumpPlanTo(ctx, session, mode, plan, txn, &out)
	if err != nil {
		return nil, err
	}
	return &Result{AffectedRows: affectedRows, RenderedOutput: out.Bytes()}, nil
}

// executeDumpStreamingWithTxn writes dump output directly to out.
// Callers that export data must pass the same read-only transaction used
// for catalog preflight. SCHEMA has no data txn.
func executeDumpStreamingWithTxn(ctx context.Context, session *Session, mode dumpMode, plan *dumpPlan, out io.Writer, txn *spanner.ReadOnlyTransaction) (*Result, error) {
	affectedRows, err := writeDumpPlanTo(ctx, session, mode, plan, txn, out)
	if err != nil {
		return nil, err
	}
	return &Result{AffectedRows: affectedRows, Streamed: true}, nil
}

// writeDumpPlanTo writes a prepared plan to out. Its caller chooses whether
// out is an outer buffer, which withholds output on failure, or a streaming
// destination, which may have received completed units before a later error.
// Data queries use the same writer and read transaction as the surrounding
// traversal, so catalog, cyclic planning, and exported values share a snapshot.
func writeDumpPlanTo(ctx context.Context, session *Session, mode dumpMode, plan *dumpPlan, txn *spanner.ReadOnlyTransaction, out io.Writer) (int, error) {
	probed := false
	probeOutput := func() {
		if session.dumpReadTxnProbe != nil && !probed {
			probed = true
			session.dumpReadTxnProbe("output", txn)
		}
	}
	if len(plan.DDL) > 0 {
		probeOutput()
		if _, err := out.Write(plan.DDL); err != nil {
			return 0, fmt.Errorf("write DDL: %w", err)
		}
	}
	if !mode.shouldExportData() {
		return 0, nil
	}
	var totalAffectedRows int
	for _, unit := range plan.Data {
		probeOutput()
		if unit.Cyclic != nil {
			if err := unit.Cyclic.writeTo(out); err != nil {
				return 0, fmt.Errorf("write cyclic data: %w", err)
			}
			totalAffectedRows += len(unit.Cyclic.Statements)
			continue
		}
		table := unit.Table
		if len(table.Columns) == 0 {
			if _, err := fmt.Fprintf(out, "-- Skipping table %s (no writable columns)\n", table.ID.FQN()); err != nil {
				return 0, fmt.Errorf("write data for table %s: %w", table.ID.FQN(), err)
			}
			continue
		}
		if unit.Empty {
			if _, err := fmt.Fprintf(out, "-- Data for table %s\n", table.ID.FQN()); err != nil {
				return 0, fmt.Errorf("write data for table %s: %w", table.ID.FQN(), err)
			}
			continue
		}
		selectQuery := buildSelectQueryWithColumns(session.systemVariables.Feature.DatabaseDialect, table.Columns, table.ID)
		if _, err := fmt.Fprintf(out, "-- Data for table %s\n", table.ID.FQN()); err != nil {
			return 0, fmt.Errorf("write data for table %s: %w", table.ID.FQN(), err)
		}
		dataResult, err := executeSQLWithFormatAndTxn(ctx, session, txn, selectQuery,
			enums.DisplayModeSQLInsert, enums.StreamingModeTrue, table.ID.FQN(), out)
		if err != nil {
			return 0, fmt.Errorf("export table %s: %w", table.ID.FQN(), err)
		}
		totalAffectedRows += dataResult.AffectedRows
		if dataResult.AffectedRows > 0 {
			if _, err := fmt.Fprintln(out); err != nil {
				return 0, fmt.Errorf("write data for table %s: %w", table.ID.FQN(), err)
			}
		}
	}
	return totalAffectedRows, nil
}

// exportDDL exports database DDL statements as pre-rendered text (kind (d)).
// Each statement is terminated with ';' and followed by a blank line, matching
// the previous per-row rendering; callers write Result.RenderedOutput directly.
func exportDDL(ctx context.Context, session *Session) (*Result, error) {
	ddl, err := session.GetDatabaseDdlFresh(ctx)
	if err != nil {
		return nil, wrapDumpGetDdlError(err)
	}
	rendered, err := renderDumpDDL(ddl.Statements, ddl.GetProtoDescriptors())
	if err != nil {
		return nil, err
	}
	return &Result{RenderedOutput: rendered}, nil
}

func renderDDLStatements(statements []string) []byte {
	var out bytes.Buffer
	fmt.Fprintln(&out, "-- Database DDL exported by spanner-mycli")
	fmt.Fprintln(&out)
	for _, stmt := range statements {
		if !strings.HasSuffix(stmt, ";") {
			stmt += ";"
		}
		fmt.Fprintln(&out, stmt)
		fmt.Fprintln(&out)
	}
	return out.Bytes()
}
