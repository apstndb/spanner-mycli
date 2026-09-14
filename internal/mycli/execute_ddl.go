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
	"github.com/apstndb/spanner-mycli/enums"
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
		if session.txn != nil && session.txn.HasAutomaticDML() {
			return nil, errors.New("there is active batch DML")
		}
		return executeDdlStatements(ctx, session, ddls)
	}
}

// replacerForProgress replaces tabs and newlines to avoid breaking progress bars.
var replacerForProgress = strings.NewReplacer(
	"\n", " ",
	"\t", " ",
)

func newProgressWithTTY(ctx context.Context, session *Session) *mpb.Progress {
	if session == nil || session.systemVariables == nil || session.systemVariables.StreamManager == nil {
		return nil
	}
	ttyStream := session.systemVariables.StreamManager.GetTtyStream()
	if ttyStream == nil {
		return nil
	}
	return mpb.NewWithContext(ctx, mpb.WithOutput(ttyStream))
}

func executeDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	if len(ddls) == 0 {
		result := &Result{}
		if session.systemVariables.Feature.EchoExecutedDDL {
			result.TableHeader = toTableHeader("Executed", "Commit Timestamp")
			result.Body = PresentationBody(nil)
		}
		return result, nil
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
		p = newProgressWithTTY(ctx, session)

		if p != nil {
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
	}

	// Snapshot repair policy for this execution. Later SET must not change
	// whether this SYNC attempt is eligible.
	kind := session.systemVariables.Feature.DefaultSequenceKind
	mode := session.systemVariables.Feature.DDLExecutionMode

	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:         session.DatabasePath(),
		Statements:       ddls,
		ProtoDescriptors: b,
	})
	if err != nil {
		teardown()
		if result, repairErr, ok := tryDefaultSequenceKindRepair(ctx, session, ddls, b, kind, nil, err); ok {
			return result, repairErr
		}
		return nil, fmt.Errorf("error on create op: %w", err)
	}

	if mode == enums.DDLExecutionModeAsync {
		session.IncrementSchemaGeneration()
		return formatAsyncDdlResult(op)
	}

	var waitDeadline time.Time
	if mode == enums.DDLExecutionModeAsyncWait {
		waitDeadline = asyncWaitDeadline(session.systemVariables.Feature.DDLAsyncWaitTimeout)
	}

	result, waitErr := waitForDdlOperation(ctx, session, op, ddls, p, bars, teardown, waitDeadline)
	if waitErr == nil || mode != enums.DDLExecutionModeSync {
		return result, waitErr
	}
	if repaired, repairErr, ok := tryDefaultSequenceKindRepair(ctx, session, ddls, b, kind, op, waitErr); ok {
		return repaired, repairErr
	}
	return result, waitErr
}

// errWaitBudgetExpired is the internal signal that ASYNC_WAIT's remaining
// budget ran out. Callers convert it to a successful operation-ID handoff.
var errWaitBudgetExpired = errors.New("DDL async wait budget expired")

// asyncWaitDeadline is the absolute ASYNC_WAIT deadline. A non-positive
// timeout is already expired. The zero Time is reserved for SYNC (no budget).
func asyncWaitDeadline(timeout time.Duration) time.Time {
	if timeout <= 0 {
		return time.Now().Add(-time.Nanosecond)
	}
	return time.Now().Add(timeout)
}

// waitForDdlOperation is the single DDL wait helper. SYNC passes a zero wait
// deadline and blocks until the LRO finishes. ASYNC_WAIT uses one remaining
// wait budget across the initial poll, later polls, and the between-poll
// wait. That budget is not the caller context: expiry cancels only the
// in-flight GetOperation RPC and is a successful handoff of the still-running
// operation ID. It does not send CancelOperation. Caller/statement
// cancellation remains an error with that operation ID. A completed failing
// LRO remains a failure.
func waitForDdlOperation(ctx context.Context, session *Session, op *adminapi.UpdateDatabaseDdlOperation, ddls []string, p *mpb.Progress, bars []*mpb.Bar, teardown func(), waitDeadline time.Time) (*Result, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	handoff := func() (*Result, error) {
		teardown()
		session.IncrementSchemaGeneration()
		return formatAsyncDdlResult(op)
	}

	pollDdl := func(pollCtx context.Context) (*databasepb.UpdateDatabaseDdlMetadata, error) {
		if err := op.Poll(pollCtx); err != nil {
			return nil, err
		}

		return op.Metadata()
	}

	finishWaitErr := func(err error) (*Result, error) {
		// A cached terminal LRO is an actual result, including cancellation
		// or deadline status codes. The wait budget only applies while the
		// operation is still pending.
		if !op.Done() && errors.Is(classifyDdlWaitError(ctx, waitDeadline, err), errWaitBudgetExpired) {
			return handoff()
		}
		teardown()
		return nil, handleDdlWaitError(session, op, err)
	}

	metadata, err := pollDdlWithWaitBudget(ctx, op, waitDeadline, pollDdl)
	if err != nil {
		return finishWaitErr(err)
	}
	updateDdlProgressBars(bars, metadata)

	if !op.Done() {
		if waitDeadlineReached(waitDeadline) {
			if ctx.Err() != nil {
				return finishWaitErr(ctx.Err())
			}
			return handoff()
		}

		for !op.Done() {
			if err := waitForNextDdlPoll(ctx, ticker.C, waitDeadline); err != nil {
				return finishWaitErr(err)
			}

			metadata, err = pollDdlWithWaitBudget(ctx, op, waitDeadline, pollDdl)
			if err != nil {
				return finishWaitErr(err)
			}
			updateDdlProgressBars(bars, metadata)
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
		result.Body = PresentationBody(slices.Collect(iterutil.ZipShortestBy(slices.Values(ddls), slices.Values(metadata.GetCommitTimestamps()),
			func(ddl string, commitTimestamp *timestamppb.Timestamp) Row {
				return toRow(ddl+";", commitTimestamp.AsTime().Format(time.RFC3339Nano))
			})))
	}

	return result, nil
}

func updateDdlProgressBars(bars []*mpb.Bar, metadata *databasepb.UpdateDatabaseDdlMetadata) {
	if metadata == nil || bars == nil {
		return
	}
	progresses := metadata.GetProgress()
	for i, progress := range progresses {
		if i >= len(bars) {
			break
		}
		bar := bars[i]
		if bar.Completed() {
			continue
		}
		bar.SetCurrent(int64(progress.ProgressPercent))
	}
}

func waitDeadlineReached(deadline time.Time) bool {
	return !deadline.IsZero() && !time.Now().Before(deadline)
}

func ddlPollContext(ctx context.Context, waitDeadline time.Time) (context.Context, context.CancelFunc, error) {
	if waitDeadline.IsZero() {
		return ctx, func() {}, nil
	}
	if ctx.Err() != nil {
		return nil, func() {}, ctx.Err()
	}
	if waitDeadlineReached(waitDeadline) {
		return nil, func() {}, errWaitBudgetExpired
	}
	pollCtx, cancel := context.WithDeadline(ctx, waitDeadline)
	return pollCtx, cancel, nil
}

func pollDdlWithWaitBudget(ctx context.Context, op *adminapi.UpdateDatabaseDdlOperation, waitDeadline time.Time, pollDdl func(context.Context) (*databasepb.UpdateDatabaseDdlMetadata, error)) (*databasepb.UpdateDatabaseDdlMetadata, error) {
	// Poll resolves a cached terminal operation without GetOperation. Do that
	// before applying the wait budget so a completed LRO is not turned into a
	// successful async handoff.
	if op.Done() {
		return pollDdl(ctx)
	}
	pollCtx, cancel, err := ddlPollContext(ctx, waitDeadline)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return pollDdl(pollCtx)
}

func waitForNextDdlPoll(ctx context.Context, ticker <-chan time.Time, waitDeadline time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitDeadlineReached(waitDeadline) {
		return errWaitBudgetExpired
	}

	var timeout <-chan time.Time
	if !waitDeadline.IsZero() {
		timer := time.NewTimer(time.Until(waitDeadline))
		defer timer.Stop()
		timeout = timer.C
	}

	select {
	case <-ticker:
		return nil
	case <-timeout:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errWaitBudgetExpired
	case <-ctx.Done():
		return ctx.Err()
	}
}

// classifyDdlWaitError maps a wait/poll error onto either the budget-expiry
// handoff sentinel or the original error. Caller/STATEMENT_TIMEOUT cancellation
// wins over a simultaneously expired wait budget.
func classifyDdlWaitError(ctx context.Context, waitDeadline time.Time, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	if errors.Is(err, errWaitBudgetExpired) {
		return errWaitBudgetExpired
	}
	if isCancellationError(err) && waitDeadlineReached(waitDeadline) {
		return errWaitBudgetExpired
	}
	return err
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

	// A completed LRO already has its real outcome, including an operation
	// that failed with Canceled or DeadlineExceeded. Those are not a local
	// wait-budget or caller-context cancellation.
	if op.Done() || !isCancellationError(err) {
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
// A completed LRO that failed with those same status codes is excluded by
// handleDdlWaitError via op.Done() so a known operation failure is not
// mistaken for expiration of the local polling context.
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
		Body:         PresentationBody(rows),
		AffectedRows: 1,
	}, nil
}
