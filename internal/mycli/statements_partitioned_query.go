package mycli

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
)

type PartitionStatement struct{ SQL string }

func (s *PartitionStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.txn.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	txBlob, err := batchROTx.ID.MarshalBinary()
	if err != nil {
		return nil, err
	}
	now := partitionTokenNow()
	database := session.DatabasePath()
	rows := make([]Row, 0, len(partitions))
	for _, partition := range partitions {
		partBlob, err := partition.MarshalBinary()
		if err != nil {
			return nil, err
		}
		token, err := encodePartitionToken(database, now, txBlob, partBlob)
		if err != nil {
			return nil, err
		}
		rows = append(rows, toRow(token))
	}

	ts, err := batchROTx.Timestamp()
	if err != nil {
		return nil, err
	}

	return &Result{
		TableHeader:   toTableHeader("Partition_Token"),
		Body:          PresentationBody(rows),
		AffectedRows:  len(rows),
		ReadTimestamp: ts,
		ForceWrap:     true,
	}, nil
}

type TryPartitionedQueryStatement struct{ SQL string }

func (s *TryPartitionedQueryStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	_, batchROTx, err := session.txn.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	ts, err := batchROTx.Timestamp()
	if err != nil {
		return nil, err
	}

	return &Result{
		TableHeader:   toTableHeader("Root_Partitionable"),
		Body:          PresentationBody(sliceOf(toRow("TRUE"))),
		AffectedRows:  1,
		ReadTimestamp: ts,
		ForceWrap:     true,
	}, nil
}

type RunPartitionedQueryStatement struct{ SQL string }

func (s *RunPartitionedQueryStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	return runPartitionedQuery(ctx, session, s.SQL, out)
}

type RunPartitionStatement struct{ Token string }

func (s *RunPartitionStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	if err := rejectRunPartitionAdmission(session); err != nil {
		return nil, err
	}

	decoded, err := decodePartitionToken(s.Token, partitionTokenNow())
	if err != nil {
		return nil, err
	}
	database := session.DatabasePath()
	if decoded.Database != database {
		return nil, fmt.Errorf("partition token database %q does not match session %q", decoded.Database, database)
	}

	inspected, err := inspectNativePartition(decoded.TxID, decoded.Partition, database)
	if err != nil {
		return nil, err
	}

	var txID spanner.BatchReadOnlyTransactionID
	if err := txID.UnmarshalBinary(decoded.TxID); err != nil {
		return nil, fmt.Errorf("malformed txid blob: %w", err)
	}
	var part spanner.Partition
	if err := part.UnmarshalBinary(decoded.Partition); err != nil {
		return nil, fmt.Errorf("malformed partition blob: %w", err)
	}

	out = session.resolveOperationOutput(out)
	sysVars := session.systemVariables
	render, err := prepareFormatConfig(inspected.SQL, sysVars, queryRenderingFrom(sysVars))
	if err != nil {
		return nil, err
	}

	batchROTx := session.client.BatchReadOnlyTransactionFromID(txID)
	defer batchROTx.Close()

	m := newMetrics(sysVars)
	result, err := executeAndCollect(ctx, &queryExecution{
		Session:        session,
		Out:            out,
		Iter:           batchROTx.Execute(ctx, &part),
		ReadOnlyTxn:    &batchROTx.ReadOnlyTransaction,
		SQL:            inspected.SQL,
		SysVars:        sysVars,
		Render:         render,
		Metrics:        m,
		QueryCacheDest: &sysVars.LastResult.QueryCache,
		Receipt:        nil,
	})
	if err != nil {
		return nil, err
	}
	result.PartitionCount = 1
	if render.ValueFmtMode == format.SQLLiteralValues && render.Export.SQLTableName != "" {
		result.SQLTableNameForExport = render.Export.SQLTableName
	}
	return result, nil
}

func rejectRunPartitionAdmission(session *Session) error {
	if session != nil && session.batch.IsActive() {
		return fmt.Errorf("RUN PARTITION requires an idle session without a manual batch; this token is an exported snapshot")
	}
	if session != nil && session.txn != nil && session.txn.InTransaction() {
		return fmt.Errorf("RUN PARTITION requires an idle session without a live transaction; this token is an exported snapshot")
	}
	return nil
}
