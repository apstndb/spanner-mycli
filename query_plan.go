//
// Copyright 2020 Google LLC
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
//

package main

import (
	"fmt"
	"strings"

	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func processPlanWithStats(plan *sppb.QueryPlan) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, true)
}

func processPlanWithoutStats(plan *sppb.QueryPlan) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, false)
}

func processPlanImpl(plan *sppb.QueryPlan, withStats bool) (rows []Row, predicates []string, err error) {
	rowsWithPredicates, err := plantree.ProcessPlan(queryplan.New(plan.GetPlanNodes()))
	if err != nil {
		return nil, nil, err
	}

	var maxIDLength int
	for _, row := range rowsWithPredicates {
		if length := len(fmt.Sprint(row.ID)); length > maxIDLength {
			maxIDLength = length
		}
	}

	for _, row := range rowsWithPredicates {
		if withStats {
			rows = append(rows, toRow(row.FormatID(), row.Text(), row.ExecutionStats.Rows.Total, row.ExecutionStats.ExecutionSummary.NumExecutions, row.ExecutionStats.Latency.String()))
		} else {
			rows = append(rows, toRow(row.FormatID(), row.Text()))
		}

		var prefix string
		for i, predicate := range row.Predicates {
			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxIDLength, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxIDLength+1)
			}
			predicates = append(predicates, fmt.Sprintf("%s %s", prefix, predicate))
		}
	}

	return rows, predicates, nil
}
