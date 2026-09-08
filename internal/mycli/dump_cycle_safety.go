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
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/iterator"
)

const dumpCyclicInsertUnsupported = "sequential INSERT is unsupported for a cyclic foreign-key or interleave dependency among"

func rejectPopulatedCyclicDumpSCCs(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, resolver *DependencyResolver, selected []tableID, dro *sppb.DirectedReadOptions) error {
	if !dumpCycleSafetyPreflight {
		return nil
	}
	sccs := resolver.cyclicSafetySCCs(selected)
	dialect := session.systemVariables.Feature.DatabaseDialect
	for _, scc := range sccs {
		for _, id := range scc {
			if session.dumpReadTxnProbe != nil {
				session.dumpReadTxnProbe("preflight", txn)
			}
			if session.dumpCyclePreflightProbe != nil {
				if err := session.dumpCyclePreflightProbe(id, txn); err != nil {
					return err
				}
			}
			hasRows, err := tableHasRowsWithTxn(ctx, txn, dialect, id, dro)
			if err != nil {
				return err
			}
			if hasRows {
				return dumpCyclicInsertError(scc)
			}
		}
	}
	return nil
}

func dumpCyclicInsertError(scc []tableID) error {
	names := make([]string, len(scc))
	for i, id := range scc {
		names[i] = id.FQN()
	}
	slices.Sort(names)
	return fmt.Errorf("%s %s; enforced foreign-key checks run after each DML statement and BEGIN does not defer them. This is an interim safety restriction; the cyclic data is not restored",
		dumpCyclicInsertUnsupported, strings.Join(names, ", "))
}

func tableHasRowsWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction, dialect dbadminpb.DatabaseDialect, id tableID, dro *sppb.DirectedReadOptions) (bool, error) {
	iter := queryWithDirectedRead(ctx, txn, spanner.Statement{
		SQL: fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", quoteTableID(dialect, id)),
	}, dro)
	defer iter.Stop()
	_, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dump cycle safety: row presence for %s: %w", id.FQN(), err)
	}
	return true, nil
}
