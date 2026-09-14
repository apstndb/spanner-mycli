// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAutocommitDMLModeFallbackEnum(t *testing.T) {
	t.Parallel()
	got, err := enums.AutocommitDMLModeString("TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC")
	if err != nil {
		t.Fatal(err)
	}
	if got != enums.AutocommitDMLModeTransactionalWithFallbackToPartitionedNonAtomic {
		t.Fatalf("got %v", got)
	}
	if enums.AutocommitDMLModeTransactional.String() != "TRANSACTIONAL" {
		t.Fatalf("default value string changed: %s", enums.AutocommitDMLModeTransactional)
	}
}

func TestAutocommitDMLModeSetLocalFallbackValue(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
	if got := mustGetVar(t, session, "AUTOCOMMIT_DML_MODE"); got != "TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC" {
		t.Fatalf("SET LOCAL = %q", got)
	}
	mustExec(t, ctx, session, "ROLLBACK")
	if got := mustGetVar(t, session, "AUTOCOMMIT_DML_MODE"); got != "TRANSACTIONAL" {
		t.Fatalf("restored = %q", got)
	}
}

func TestIsMutationLimitExceededClassifier(t *testing.T) {
	t.Parallel()

	strong := wrapAsAbortedAPIError(t, mutationLimitStatusErr(t))
	requireAPIError(t, strong)
	if !isMutationLimitExceeded(strong) {
		t.Fatal("strong Help through *apierror.APIError must match")
	}

	handmade := spanner.ToSpannerError(status.Error(codes.InvalidArgument, mutationLimitSentence))
	if isMutationLimitExceeded(handmade) {
		t.Fatal("handmade spanner.Error without Help must not match")
	}
	if st, ok := status.FromError(handmade); ok && len(st.Details()) > 0 {
		t.Fatal("handmade status.Error unexpectedly carried details")
	}

	for _, tc := range []struct {
		name string
		err  error
		// Each named negative must pass every earlier classifier gate so it
		// actually exercises the intended predicate.
		wantCode     codes.Code
		wantSentence bool
		wantAPIErr   bool
	}{
		{
			name:         "wrong Help URL",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithHelp(t, mutationLimitHelpDesc, "https://example.invalid/limits")),
			wantCode:     codes.InvalidArgument,
			wantSentence: true,
			wantAPIErr:   true,
		},
		{
			name:         "wrong Help description",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithHelp(t, "Wrong documentation.", mutationLimitHelpURL)),
			wantCode:     codes.InvalidArgument,
			wantSentence: true,
			wantAPIErr:   true,
		},
		{
			name:         "extra Help link",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithExtraHelp(t)),
			wantCode:     codes.InvalidArgument,
			wantSentence: true,
			wantAPIErr:   true,
		},
		{
			name:         "weaker resource-limits text",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithDesc(t, weakerResourceLimitsMessage())),
			wantCode:     codes.InvalidArgument,
			wantSentence: false,
			wantAPIErr:   true,
		},
		{
			name:         "ResourceExhausted",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithCode(t, codes.ResourceExhausted, mutationLimitSentence)),
			wantCode:     codes.ResourceExhausted,
			wantSentence: true,
			wantAPIErr:   true,
		},
		{
			name:         "Aborted",
			err:          wrapAsAbortedAPIError(t, mutationLimitStatusWithCode(t, codes.Aborted, mutationLimitSentence)),
			wantCode:     codes.Aborted,
			wantSentence: true,
			wantAPIErr:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := spanner.ErrCode(tc.err); got != tc.wantCode {
				t.Fatalf("ErrCode = %v, want %v", got, tc.wantCode)
			}
			if got := strings.Contains(spanner.ErrDesc(tc.err), mutationLimitSentence); got != tc.wantSentence {
				t.Fatalf("sentence present = %v, want %v", got, tc.wantSentence)
			}
			if tc.wantAPIErr {
				requireAPIError(t, tc.err)
			}
			if isMutationLimitExceeded(tc.err) {
				t.Fatal("must not match")
			}
		})
	}

	if isMutationLimitExceeded(errors.New(mutationLimitSentence)) {
		t.Fatal("message-only error must not match")
	}
	if isMutationLimitExceeded(nil) {
		t.Fatal("nil must not match")
	}
}

func wrapAsAbortedAPIError(t *testing.T, err error) error {
	t.Helper()
	apiErr, ok := apierror.FromError(err)
	if !ok {
		t.Fatalf("apierror.FromError(%T) failed", err)
	}
	return fmt.Errorf("transaction was aborted: %w", apiErr)
}

func requireAPIError(t *testing.T, err error) {
	t.Helper()
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("missing *apierror.APIError: %T %v", err, err)
	}
}

func weakerResourceLimitsMessage() string {
	return "Transaction resource limits exceeded"
}

func mutationLimitStatusErr(t *testing.T) error {
	t.Helper()
	return mutationLimitStatusWithHelp(t, mutationLimitHelpDesc, mutationLimitHelpURL)
}

func mutationLimitStatusWithDesc(t *testing.T, desc string) error {
	t.Helper()
	return mutationLimitStatusWithHelpAndDesc(t, desc, mutationLimitHelpDesc, mutationLimitHelpURL)
}

func mutationLimitStatusWithHelp(t *testing.T, helpDesc, helpURL string) error {
	t.Helper()
	return mutationLimitStatusWithHelpAndDesc(t, mutationLimitSentence, helpDesc, helpURL)
}

func mutationLimitStatusWithHelpAndDesc(t *testing.T, desc, helpDesc, helpURL string) error {
	t.Helper()
	st, err := status.New(codes.InvalidArgument, desc).WithDetails(&errdetails.Help{
		Links: []*errdetails.Help_Link{{
			Description: helpDesc,
			Url:         helpURL,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

func mutationLimitStatusWithExtraHelp(t *testing.T) error {
	t.Helper()
	st, err := status.New(codes.InvalidArgument, mutationLimitSentence).WithDetails(&errdetails.Help{
		Links: []*errdetails.Help_Link{
			{Description: mutationLimitHelpDesc, Url: mutationLimitHelpURL},
			{Description: "extra", Url: "https://example.invalid/extra"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

func mutationLimitStatusWithCode(t *testing.T, code codes.Code, desc string) error {
	t.Helper()
	return mutationLimitStatusWithCodeAndHelp(t, code, desc, mutationLimitHelpDesc, mutationLimitHelpURL)
}

func mutationLimitStatusWithCodeAndHelp(t *testing.T, code codes.Code, desc, helpDesc, helpURL string) error {
	t.Helper()
	st, err := status.New(code, desc).WithDetails(&errdetails.Help{
		Links: []*errdetails.Help_Link{{
			Description: helpDesc,
			Url:         helpURL,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

func TestCaptureMutationLimitFallbackEligibility(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	session.systemVariables.Transaction.AutocommitDMLMode = enums.AutocommitDMLModeTransactionalWithFallbackToPartitionedNonAtomic
	if got := captureMutationLimitFallback(session, "UPDATE T SET v = 1 WHERE TRUE"); got == nil {
		t.Fatal("idle UPDATE should be eligible")
	}
	if got := captureMutationLimitFallback(session, "DELETE FROM T WHERE TRUE"); got == nil {
		t.Fatal("idle DELETE should be eligible")
	}
	if got := captureMutationLimitFallback(session, "INSERT INTO T (id) VALUES (1)"); got != nil {
		t.Fatal("INSERT must not be eligible")
	}
	if got := captureMutationLimitFallback(session, "UPDATE T SET v = 1 WHERE TRUE THEN RETURN v"); got != nil {
		t.Fatal("THEN RETURN must not be eligible")
	}
	session.systemVariables.Transaction.AutocommitDMLMode = enums.AutocommitDMLModeTransactional
	if got := captureMutationLimitFallback(session, "UPDATE T SET v = 1 WHERE TRUE"); got != nil {
		t.Fatal("default TRANSACTIONAL must not be eligible")
	}
}

func TestWrapMutationLimitFallbackError(t *testing.T) {
	t.Parallel()
	orig := errors.New("orig")
	fallback := errors.New("pdml")
	err := wrapMutationLimitFallbackError(orig, fallback)
	if !errors.Is(err, orig) || !errors.Is(err, fallback) {
		t.Fatalf("wrapped causes: %v", err)
	}
	if !strings.Contains(err.Error(), "may have partially committed") {
		t.Fatalf("missing partial-commit warning: %v", err)
	}
	if strings.Contains(err.Error(), "rolled back that phase") {
		t.Fatalf("must not claim PDML rollback: %v", err)
	}
}
