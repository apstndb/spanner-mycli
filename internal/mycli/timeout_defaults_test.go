// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/samber/lo"
)

func timeoutSession(t *testing.T, args []string, configFiles ...string) *Session {
	t.Helper()
	gopts, err := parseTestFlags(args, configFiles...)
	if err != nil {
		t.Fatalf("parseTestFlags: %v", err)
	}
	if err := ValidateSpannerOptions(&gopts.Spanner); err != nil {
		t.Fatalf("ValidateSpannerOptions: %v", err)
	}
	sv, err := initializeSystemVariables(&gopts.Spanner)
	if err != nil {
		t.Fatalf("initializeSystemVariables: %v", err)
	}
	session := &Session{
		mode:            DatabaseConnected,
		systemVariables: sv,
		txn:             NewTransactionManager(nil, sv, defaultClientConfig),
	}
	sv.inTransaction = session.txn.InTransaction
	return session
}

func writeTimeoutTOML(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "timeout.toml")
	if err := os.WriteFile(path, []byte("timeout = \""+value+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertStmtTimeout(t *testing.T, session *Session, sql string, want time.Duration) {
	t.Helper()
	stmt, err := BuildStatement(sql)
	if err != nil {
		t.Fatalf("BuildStatement(%q): %v", sql, err)
	}
	if got := session.getTimeoutForStatement(stmt); got != want {
		t.Fatalf("%s: timeout=%s want=%s var=%v type=%T", sql, got, want, session.systemVariables.Query.StatementTimeout, stmt)
	}
}

func TestOmittedTimeoutLeavesStatementTimeoutNull(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags())
	if session.systemVariables.Query.StatementTimeout != nil {
		t.Fatalf("omitted --timeout set STATEMENT_TIMEOUT to %v", *session.systemVariables.Query.StatementTimeout)
	}
	assertStmtTimeout(t, session, "SELECT 1", 10*time.Minute)
	assertStmtTimeout(t, session, "PARTITIONED UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)
}

func TestExplicitTenMinutesIsNotOmission(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--timeout", "10m"))
	if session.systemVariables.Query.StatementTimeout == nil || *session.systemVariables.Query.StatementTimeout != 10*time.Minute {
		t.Fatalf("explicit 10m: %v", session.systemVariables.Query.StatementTimeout)
	}
	assertStmtTimeout(t, session, "SELECT 1", 10*time.Minute)
	assertStmtTimeout(t, session, "PARTITIONED UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
}

func TestAutomaticPDMLUses24hWhenTimeoutNull(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--enable-partitioned-dml", "--set", "STATEMENT_TIMEOUT=NULL"))
	if session.systemVariables.Query.StatementTimeout != nil {
		t.Fatal("expected NULL STATEMENT_TIMEOUT")
	}
	assertStmtTimeout(t, session, "SELECT 1", 10*time.Minute)
	assertStmtTimeout(t, session, "INSERT INTO T (Id) VALUES (1)", 10*time.Minute)
	assertStmtTimeout(t, session, "insert into T (Id) VALUES (1)", 10*time.Minute)
	assertStmtTimeout(t, session, "@{JOIN_METHOD=HASH_JOIN} INSERT INTO T (Id) VALUES (1)", 10*time.Minute)
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)
	assertStmtTimeout(t, session, "DELETE FROM T WHERE TRUE", 24*time.Hour)
	assertStmtTimeout(t, session, "PARTITIONED UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)
}

func TestTimeoutConfigurationMatrix(t *testing.T) {
	t.Parallel()
	toml45 := writeTimeoutTOML(t, "45s")
	for _, tc := range []struct {
		name   string
		args   []string
		config []string
		custom *time.Duration
	}{
		{name: "omitted"},
		{name: "explicit_10m", args: []string{"--timeout", "10m"}, custom: lo.ToPtr(10 * time.Minute)},
		{name: "explicit_30s", args: []string{"--timeout", "30s"}, custom: lo.ToPtr(30 * time.Second)},
		{name: "zero", args: []string{"--timeout", "0s"}, custom: lo.ToPtr(time.Duration(0))},
		{name: "toml_45s", config: []string{toml45}, custom: lo.ToPtr(45 * time.Second)},
		{name: "flag_over_toml", args: []string{"--timeout", "30s"}, config: []string{toml45}, custom: lo.ToPtr(30 * time.Second)},
		{name: "set_over_flag", args: []string{"--timeout", "30s", "--set", "STATEMENT_TIMEOUT=2m"}, custom: lo.ToPtr(2 * time.Minute)},
		{name: "set_null", args: []string{"--set", "STATEMENT_TIMEOUT=NULL"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := timeoutSession(t, append(withRequiredFlags("--enable-partitioned-dml"), tc.args...), tc.config...)
			wantOrdinary := 10 * time.Minute
			wantPDML := 24 * time.Hour
			if tc.custom != nil {
				wantOrdinary = *tc.custom
				wantPDML = *tc.custom
			}
			assertStmtTimeout(t, session, "SELECT 1", wantOrdinary)
			assertStmtTimeout(t, session, "INSERT INTO T (Id) VALUES (1)", wantOrdinary)
			assertStmtTimeout(t, session, "PARTITIONED UPDATE T SET V = 1 WHERE TRUE", wantPDML)
			assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", wantPDML)
			assertStmtTimeout(t, session, "DELETE FROM T WHERE TRUE", wantPDML)
		})
	}
}

func TestTimeoutInteractiveNullAndSetLocal(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--enable-partitioned-dml", "--timeout", "30s"))
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "STATEMENT_TIMEOUT", Value: "NULL"}); err != nil {
		t.Fatal(err)
	}
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "STATEMENT_TIMEOUT", Value: "'45s'"}); err != nil {
		t.Fatal(err)
	}
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 45*time.Second)
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if session.systemVariables.Query.StatementTimeout != nil {
		t.Fatalf("SET LOCAL leaked: %v", *session.systemVariables.Query.StatementTimeout)
	}
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)
}

func TestTimeoutPendingTransactionAndManualBatchStayOrdinary(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--enable-partitioned-dml"))
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}

	if err := session.batch.Start(batchModeDML); err != nil {
		t.Fatal(err)
	}
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
	session.batch.Abort()
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 24*time.Hour)
}

func TestTimeoutQueryModeAndTryPartitionStayOrdinary(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--enable-partitioned-dml"))
	session.systemVariables.Query.TryPartitionQuery = true
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
	session.systemVariables.Query.TryPartitionQuery = false
	session.systemVariables.Query.QueryMode = lo.ToPtr(sppb.ExecuteSqlRequest_PLAN)
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
	session.systemVariables.Query.QueryMode = lo.ToPtr(sppb.ExecuteSqlRequest_PROFILE)
	assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
}

func TestNegativeTimeoutStillRejected(t *testing.T) {
	t.Parallel()
	gopts, err := parseTestFlags(withRequiredFlags("--timeout", "-1s"))
	if err != nil {
		return
	}
	if _, err := initializeSystemVariables(&gopts.Spanner); err == nil {
		t.Fatal("negative timeout accepted")
	}
}

func TestAutocommitUsesPartitionedDMLPredicate(t *testing.T) {
	t.Parallel()
	session := timeoutSession(t, withRequiredFlags("--enable-partitioned-dml"))
	if !autocommitUsesPartitionedDML(session, "UPDATE T SET V = 1 WHERE TRUE") {
		t.Fatal("expected autocommit PDML for UPDATE")
	}
	if autocommitUsesPartitionedDML(session, "INSERT INTO T (Id) VALUES (1)") {
		t.Fatal("INSERT must not use autocommit PDML")
	}
	session.systemVariables.Transaction.AutocommitDMLMode = enums.AutocommitDMLModeTransactional
	if autocommitUsesPartitionedDML(session, "UPDATE T SET V = 1 WHERE TRUE") {
		t.Fatal("transactional autocommit must not use PDML")
	}
}
