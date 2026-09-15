// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"

	"github.com/apstndb/spancodec"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
)

// ShowHistoryStatement lists recorded statements from the live interactive
// history, or from CLI_HISTORY_FILE when that live handle is unset.
type ShowHistoryStatement struct {
	// Limit is a recency cap. 0 means unlimited.
	Limit int
}

func (s *ShowHistoryStatement) isDetachedCompatible() {}

func (s *ShowHistoryStatement) allowedDuringSavepointRecovery() {}

var (
	_ DetachedCompatible             = (*ShowHistoryStatement)(nil)
	_ savepointRecoverySafeStatement = (*ShowHistoryStatement)(nil)
)

type historyRow struct {
	Statement string `spanner:"statement"`
}

var historyRowEncoder = spancodec.MustNewRowEncoder[historyRow]()

func (s *ShowHistoryStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	var sysVars *systemVariables
	if session != nil {
		sysVars = session.systemVariables
	}

	items, err := listHistoryRows(sysVars, s.Limit)
	if err != nil {
		return nil, err
	}

	result, err := executeStructRows(historyRowEncoder, items, session, out)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

func listHistoryRows(sysVars *systemVariables, limit int) ([]historyRow, error) {
	h, err := historyForShow(sysVars)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, nil
	}

	n := h.Len()
	start := 0
	if limit > 0 && n > limit {
		start = n - limit
	}

	items := make([]historyRow, 0, n-start)
	for i := start; i < n; i++ {
		items = append(items, historyRow{Statement: h.At(i)})
	}
	return items, nil
}

// historyForShow returns the live interactive History when RunInteractive has
// attached it. Otherwise it loads CLI_HISTORY_FILE through the existing
// persistentHistory loader and never calls Add.
func historyForShow(sysVars *systemVariables) (History, error) {
	if sysVars == nil {
		return nil, nil
	}
	if sysVars.interactiveHistory != nil {
		return sysVars.interactiveHistory, nil
	}
	return newPersistentHistory(sysVars.Display.HistoryFile, simplehistory.New())
}
