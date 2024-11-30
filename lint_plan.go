package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ngicks/go-iterator-helper/x/exp/xiter"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplanviz/queryplan"
)

func lookupVar(varToExp map[string]*sppb.PlanNode, ref string) string {
	if !strings.HasPrefix(ref, "$") {
		return ref
	}

	if v, ok := varToExp[strings.TrimPrefix(ref, "$")]; ok {
		return lookupVar(varToExp, v.GetShortRepresentation().GetDescription())
	}

	return ref
}

func descToKeyElem(varToExp map[string]*sppb.PlanNode, desc string) string {
	first, last, found := strings.Cut(desc, " ")
	keyElem := lookupVar(varToExp, first)
	if found {
		keyElem = keyElem + " " + strings.TrimSuffix(strings.TrimPrefix(last, "("), ")")
	}
	return keyElem
}

func buildVariableToNodeMap(qp *queryplan.QueryPlan) map[string]*sppb.PlanNode {
	variableToExp := make(map[string]*sppb.PlanNode)
	for _, row := range qp.PlanNodes() {
		for _, cl := range row.GetChildLinks() {
			if cl.GetVariable() != "" {
				variableToExp[cl.GetVariable()] = qp.GetNodeByChildLink(cl)
			}
		}
	}
	return variableToExp
}

func LinkTypePred(typ string) func(cl *sppb.PlanNode_ChildLink) bool {
	return func(cl *sppb.PlanNode_ChildLink) bool {
		return cl.GetType() == typ
	}
}

func formatKeyElem(qp *queryplan.QueryPlan, variableToExp map[string]*sppb.PlanNode) func(cl *sppb.PlanNode_ChildLink) string {
	return func(cl *sppb.PlanNode_ChildLink) string {
		return descToKeyElem(variableToExp, qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription())
	}
}

func lintPlan(plan *sppb.QueryPlan) []string {
	qp := queryplan.New(plan.GetPlanNodes())
	variableToExp := buildVariableToNodeMap(qp)

	var result []string
	for _, planNode := range qp.PlanNodes() {
		var msgs []string

		// Process display name
		switch {
		case planNode.GetDisplayName() == "Filter":
			msgs = append(msgs, "Potentially expensive operator Filter can't utilize index: Maybe better to modify to use Filter Scan with Seek Condition?")
		case planNode.GetDisplayName() == "Hash Join":
			msgs = append(msgs, "Potentially expensive operator Hash Join: Maybe better to modify to use Cross Apply or Merge Join?")
		case strings.Contains(planNode.GetDisplayName(), "Minor Sort"):
			msgs = append(msgs, fmt.Sprintf("Potentially expensive operator Minor Sort is cheaper than Sort but it may be not optimal: Maybe better to modify to use the same order with the index?: major: %v, minor: %v",
				strings.Join(formatKeyElemForLinkType(qp, variableToExp, planNode, "MajorKey"), ", "),
				strings.Join(formatKeyElemForLinkType(qp, variableToExp, planNode, "MinorKey"), ", "),
			))
		case strings.Contains(planNode.GetDisplayName(), "Sort"):
			msgs = append(msgs, fmt.Sprintf("Potentially expensive operator Sort: Maybe better to modify to use the same order with the index?, order: %v",
				strings.Join(formatKeyElemForLinkType(qp, variableToExp, planNode, "Key"), ", ")))
		}

		// Process child links
		for _, childLink := range planNode.GetChildLinks() {
			var msg string
			switch {
			case childLink.GetType() == "Residual Condition":
				msg = "Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition"
			}

			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v: %v", childLink.GetType(), msg))
			}
		}

		// process metadata
		for k, v := range planNode.GetMetadata().AsMap() {
			var msg string
			switch {
			case k == "Full scan" && v == "true":
				msg = "Potentially expensive execution full scan: Do you really want full scan?"
			case k == "iterator_type" && v == "Hash":
				msg = fmt.Sprintf("Potentially expensive execution Hash %s: Maybe better to modify to use Stream %s?, keys: %v", planNode.GetDisplayName(), planNode.GetDisplayName(),
					strings.Join(formatKeyElemForLinkType(qp, variableToExp, planNode, "Key"), ", "))
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v=%v: %v", k, v, msg))
			}
		}

		if len(msgs) > 0 {
			result = append(result, fmt.Sprintf("%v: %v", planNode.GetIndex(), queryplan.NodeTitle(planNode)))
			for _, msg := range msgs {
				result = append(result, fmt.Sprintf("    %v", msg))
			}
		}
	}
	return result
}

func formatKeyElemForLinkType(qp *queryplan.QueryPlan, variableToExp map[string]*sppb.PlanNode, node *sppb.PlanNode, linkType string) []string {
	return slices.Collect(xiter.Map(
		formatKeyElem(qp, variableToExp),
		xiter.Filter(LinkTypePred(linkType), slices.Values(node.GetChildLinks()))))
}
