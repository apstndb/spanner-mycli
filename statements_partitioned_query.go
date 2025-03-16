package main

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
)

type PartitionStatement struct{ SQL string }

func (s *PartitionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	rows := slices.Collect(xiter.Map(
		func(partition *spanner.Partition) Row {
			return toRow(base64.StdEncoding.EncodeToString(partition.GetPartitionToken()))
		},
		slices.Values(partitions)))

	ts, err := batchROTx.Timestamp()
	if err != nil {
		return nil, err
	}

	return &Result{
		ColumnNames:  sliceOf("Partition_Token"),
		Rows:         rows,
		AffectedRows: len(rows),
		Timestamp:    ts,
		ForceWrap:    true,
	}, nil
}

type TryPartitionedQueryStatement struct{ SQL string }

func (s *TryPartitionedQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	_, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
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
		ColumnNames:  sliceOf("Root_Partitionable"),
		Rows:         sliceOf(toRow("TRUE")),
		AffectedRows: 1,
		Timestamp:    ts,
		ForceWrap:    true,
	}, nil
}

type RunPartitionedQueryStatement struct{ SQL string }

func (s *RunPartitionedQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	fc, err := formatConfigWithProto(session.systemVariables.ProtoDescriptor, session.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	var allRows []Row
	var rowType *sppb.StructType
	for _, partition := range partitions {
		iter := batchROTx.Execute(ctx, partition)
		rows, _, _, md, _, err := consumeRowIterCollect(iter, spannerRowToRow(fc))
		if err != nil {
			return nil, err
		}
		allRows = append(allRows, rows...)

		if len(md.GetRowType().GetFields()) > 0 {
			rowType = md.GetRowType()
		}
	}

	result := &Result{
		ColumnNames:    extractColumnNames(rowType.GetFields()),
		Rows:           allRows,
		ColumnTypes:    rowType.GetFields(),
		AffectedRows:   len(allRows),
		PartitionCount: len(partitions),
	}
	return result, nil
}

type RunPartitionStatement struct{ Token string }

func (s *RunPartitionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return nil, errors.New("unsupported statement")
}
