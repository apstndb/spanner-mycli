package main

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"

	"cloud.google.com/go/spanner"
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
		TableHeader:  toTableHeader("Partition_Token"),
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
		TableHeader:  toTableHeader("Root_Partitionable"),
		Rows:         sliceOf(toRow("TRUE")),
		AffectedRows: 1,
		Timestamp:    ts,
		ForceWrap:    true,
	}, nil
}

type RunPartitionedQueryStatement struct{ SQL string }

func (s *RunPartitionedQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return runPartitionedQuery(ctx, session, s.SQL)
}

type RunPartitionStatement struct{ Token string }

func (s *RunPartitionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return nil, errors.New("unsupported statement")
}
