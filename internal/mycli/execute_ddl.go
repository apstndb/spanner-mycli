package mycli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/lox"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/samber/lo"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
)

func bufferOrExecuteDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	switch b := session.currentBatch.(type) {
	case *BatchDMLStatement:
		return nil, errors.New("there is active batch DML")
	case *BulkDdlStatement:
		b.Ddls = append(b.Ddls, ddls...)
		return &Result{}, nil
	default:
		return executeDdlStatements(ctx, session, ddls)
	}
}

// replacerForProgress replaces tabs and newlines to avoid breaking progress bars.
var replacerForProgress = strings.NewReplacer(
	"\n", " ",
	"\t", " ",
)

func executeDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	if len(ddls) == 0 {
		return &Result{
			TableHeader: toTableHeader(lox.IfOrEmpty(session.systemVariables.Feature.EchoExecutedDDL, sliceOf("Executed", "Commit Timestamp"))),
		}, nil
	}

	b, err := proto.Marshal(session.systemVariables.Internal.ProtoDescriptor)
	if err != nil {
		return nil, err
	}

	var p *mpb.Progress
	var bars []*mpb.Bar
	teardown := func() {
		for _, bar := range bars {
			bar.Abort(true)
		}
		if p != nil {
			p.Wait()
		}
	}
	if session.systemVariables.Display.EnableProgressBar {
		p = mpb.NewWithContext(ctx)

		for _, ddl := range ddls {
			bar := p.AddBar(int64(100),
				mpb.PrependDecorators(
					decor.Spinner(nil, decor.WCSyncSpaceR),
					decor.Name(runewidth.Truncate(replacerForProgress.Replace(ddl), 40, "..."), decor.WCSyncSpaceR),
					decor.Percentage(decor.WCSyncSpace),
					decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace)),
				mpb.BarRemoveOnComplete(),
			)
			bar.EnableTriggerComplete()
			bars = append(bars, bar)
		}
	}

	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:         session.DatabasePath(),
		Statements:       ddls,
		ProtoDescriptors: b,
	})
	if err != nil {
		teardown()
		return nil, fmt.Errorf("error on create op: %w", err)
	}

	// If async mode is enabled, return operation info immediately
	// This allows the client to continue without waiting for the DDL operation to complete.
	// In async DDL, errors are reported when polling, not immediately available.
	if session.systemVariables.Feature.AsyncDDL {
		session.IncrementSchemaGeneration()
		return formatAsyncDdlResult(op)
	}

	for !op.Done() {
		time.Sleep(5 * time.Second)
		err := op.Poll(ctx)
		if err != nil {
			teardown()
			return nil, err
		}

		metadata, err := op.Metadata()
		if err != nil {
			teardown()
			return nil, err
		}

		if bars != nil {
			progresses := metadata.GetProgress()
			for i, progress := range progresses {
				bar := bars[i]
				if bar.Completed() {
					continue
				}
				progressPercent := int64(progress.ProgressPercent)
				bar.SetCurrent(progressPercent)
			}
		}
	}

	metadata, err := op.Metadata()
	if err != nil {
		teardown()
		return nil, err
	}

	if p != nil {
		// force bars are completed even if in emulator
		for _, bar := range bars {
			if bar.Completed() {
				continue
			}
			bar.SetCurrent(100)
		}

		p.Wait()
	}

	session.IncrementSchemaGeneration()

	lastCommitTS := lo.LastOrEmpty(metadata.CommitTimestamps).AsTime()
	result := &Result{CommitTimestamp: lastCommitTS}
	if session.systemVariables.Feature.EchoExecutedDDL {
		result.TableHeader = toTableHeader("Executed", "Commit Timestamp")
		result.Rows = slices.Collect(hiter.Unify(
			func(ddl string, v *timestamppb.Timestamp) Row {
				return toRow(ddl+";", v.AsTime().Format(time.RFC3339Nano))
			},
			hiter.Pairs(slices.Values(ddls), slices.Values(metadata.GetCommitTimestamps())),
		),
		)
	}

	return result, nil
}

// formatAsyncDdlResult formats the async DDL operation result in the same format as SHOW OPERATION
func formatAsyncDdlResult(op *adminapi.UpdateDatabaseDdlOperation) (*Result, error) {
	// Get the metadata from the operation
	metadata, err := op.Metadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get operation metadata: %w", err)
	}

	operationId := lo.LastOrEmpty(strings.Split(op.Name(), "/"))

	// Use the same formatting logic as SHOW OPERATION statement
	// For async DDL, errors are reported when polling, not immediately available
	rows := formatUpdateDatabaseDdlRows(operationId, metadata, op.Done(), "")

	return &Result{
		TableHeader:  toTableHeader("OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"),
		Rows:         rows,
		AffectedRows: 1,
	}, nil
}
