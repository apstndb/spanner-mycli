package mycli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/spanner-mycli/internal/mycli/iterutil"
	"github.com/samber/lo"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
)

func bufferOrExecuteDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	switch b := session.batch.Current().(type) {
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
			TableHeader: toTableHeader(lo.Ternary(session.systemVariables.Feature.EchoExecutedDDL, sliceOf("Executed", "Commit Timestamp"), lo.Empty[[]string]())),
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
					decor.Name(tabwrap.Truncate(replacerForProgress.Replace(ddl), 40, "..."), decor.WCSyncSpaceR),
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

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	pollDdl := func() (*databasepb.UpdateDatabaseDdlMetadata, error) {
		if err := op.Poll(ctx); err != nil {
			return nil, err
		}

		return op.Metadata()
	}

	metadata, err := pollDdl()
	if err != nil {
		teardown()
		return nil, handleDdlWaitError(session, op, err)
	}

	if metadata != nil && bars != nil {
		progresses := metadata.GetProgress()
		for i, progress := range progresses {
			if i >= len(bars) {
				break
			}
			bar := bars[i]
			if bar.Completed() {
				continue
			}
			progressPercent := int64(progress.ProgressPercent)
			bar.SetCurrent(progressPercent)
		}
	}

	for !op.Done() {
		select {
		case <-ticker.C:
			// continue
		case <-ctx.Done():
			teardown()
			return nil, handleDdlWaitError(session, op, ctx.Err())
		}

		metadata, err = pollDdl()
		if err != nil {
			teardown()
			return nil, handleDdlWaitError(session, op, err)
		}

		if metadata != nil && bars != nil {
			progresses := metadata.GetProgress()
			for i, progress := range progresses {
				if i >= len(bars) {
					break
				}
				bar := bars[i]
				if bar.Completed() {
					continue
				}
				progressPercent := int64(progress.ProgressPercent)
				bar.SetCurrent(progressPercent)
			}
		}
	}

	metadata, err = op.Metadata()
	if err != nil {
		teardown()
		return nil, handleDdlWaitError(session, op, err)
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
		result.Rows = slices.Collect(iterutil.ZipShortestBy(slices.Values(ddls), slices.Values(metadata.GetCommitTimestamps()),
			func(ddl string, commitTimestamp *timestamppb.Timestamp) Row {
				return toRow(ddl+";", commitTimestamp.AsTime().Format(time.RFC3339Nano))
			}))
	}

	return result, nil
}

// handleDdlWaitError post-processes an error that terminated the synchronous DDL wait loop,
// after the UpdateDatabaseDdl operation was already accepted by the server.
//
// Two independent concerns are handled here, deliberately kept separate:
//
//   - Schema-cache invalidation is unconditional. Once the operation is accepted, ANY error exit
//     may leave the schema changed server-side: a canceled wait whose operation later completes,
//     or a multi-statement batch that fails at statement k after statements 1..k-1 already
//     committed. Invalidation is cheap and always safe, and the schema-cache TTL bounds any
//     residual staleness, so we always bump the generation rather than trying to prove the schema
//     is unchanged.
//   - Error-message classification is conditional. Only for cancellation/deadline errors do we
//     replace the raw error with a hint that surfaces the still-running operation ID via
//     SHOW OPERATION; genuine DDL failures are returned unchanged.
func handleDdlWaitError(session *Session, op *adminapi.UpdateDatabaseDdlOperation, err error) error {
	session.IncrementSchemaGeneration()

	if !isCancellationError(err) {
		return err
	}
	return ddlCancellationError(op.Name(), err)
}

// isCancellationError reports whether err represents a canceled or deadline-exceeded wait.
//
// It checks both the standard-library sentinels via errors.Is (matched when the context error is
// wrapped) AND the gRPC/Spanner status codes. The status-code check is necessary because when the
// context is canceled while a GetOperation poll RPC is in flight, grpc-go returns a plain status
// error (codes.Canceled / "context canceled") that does NOT wrap context.Canceled, so errors.Is
// alone would miss it and the cancellation hint would silently not fire.
//
// Tradeoff: a DDL that genuinely fails server-side could in principle carry codes.Canceled or
// codes.DeadlineExceeded and be misreported as a user cancellation. This is accepted because the
// in-flight-cancellation window is only reachable through those status codes, so code-based
// classification is the only way to catch it; misclassification only changes the error text (the
// invalidation above happens regardless).
func isCancellationError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch status.Code(err) {
	case codes.Canceled, codes.DeadlineExceeded:
		return true
	}
	switch spanner.ErrCode(err) {
	case codes.Canceled, codes.DeadlineExceeded:
		return true
	}
	return false
}

// ddlCancellationError builds the user-facing error for a canceled DDL wait. It extracts the
// operation ID from the full operation name (same formatting as the async DDL path) and points
// the user at SHOW OPERATION so they can attach to the still-running operation later. The
// underlying cause is wrapped so errors.Is(err, context.Canceled) still holds for callers.
func ddlCancellationError(opName string, cause error) error {
	operationID := lo.LastOrEmpty(strings.Split(opName, "/"))
	return fmt.Errorf("stopped waiting for DDL to complete; the operation may still be running server-side, check it with: SHOW OPERATION '%s': %w", operationID, cause)
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
