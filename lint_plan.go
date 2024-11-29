package main

import (
	"fmt"
	"strings"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplanviz/queryplan"
)

func lintPlan(plan *spannerpb.QueryPlan) []string {
	var result []string
	qp := queryplan.New(plan.GetPlanNodes())
	for _, row := range qp.PlanNodes() {
		var msgs []string
		switch {
		case row.GetDisplayName() == "Filter":
			msgs = append(msgs, "Potentially expensive operator Filter can't utilize index: Maybe better to modify to use Filter Scan with Seek Condition?")
		case strings.Contains(row.GetDisplayName(), "Minor Sort"):
			msgs = append(msgs, "Potentially expensive operator Minor Sort is cheaper than Sort but it may be not optimal: Maybe better to modify to use the same order with the index?")
		case strings.Contains(row.GetDisplayName(), "Sort"):
			msgs = append(msgs, "Potentially expensive operator Sort: Maybe better to modify to use the same order with the index?")
		}
		for _, childLink := range row.GetChildLinks() {
			var msg string
			switch {
			case childLink.GetType() == "Residual Condition":
				msg = "Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition"
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v: %v", childLink.GetType(), msg))
			}
		}
		for k, v := range row.GetMetadata().AsMap() {
			var msg string
			switch {
			case k == "Full scan" && v == "true":
				msg = "Potentially expensive execution full scan: Do you really want full scan?"
			case k == "iterator_type" && v == "Hash":
				msg = fmt.Sprintf("Potentially expensive execution Hash %s: Maybe better to modify to use Stream %s?", row.GetDisplayName(), row.GetDisplayName())
			case k == "join_type" && v == "Hash":
				msg = fmt.Sprintf("Potentially expensive execution Hash %s: Maybe better to modify to use Cross Apply or Merge Join?", row.GetDisplayName())
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v=%v: %v", k, v, msg))
			}
		}
		if len(msgs) > 0 {
			result = append(result, fmt.Sprintf("%v: %v", row.GetIndex(), queryplan.NodeTitle(row)))
			for _, msg := range msgs {
				result = append(result, fmt.Sprintf("    %v", msg))
			}
		}
	}
	return result
}
