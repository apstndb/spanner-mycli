package main

import (
	"fmt"

	"golang.org/x/exp/constraints"
)

func projectPath(projectID string) string {
	return fmt.Sprintf("projects/%v", projectID)
}

func instancePath(projectID, instanceID string) string {
	return fmt.Sprintf("projects/%v/instances/%v", projectID, instanceID)
}

func databasePath(projectID, instanceID, databaseID string) string {
	return fmt.Sprintf("projects/%v/instances/%v/databases/%v", projectID, instanceID, databaseID)
}

func instanceOf[T any](v any) bool {
	_, ok := v.(T)
	return ok
}

func ToSortFunc[T any, R constraints.Ordered](f func(T) R) func(T, T) int {
	return func(lhs T, rhs T) int {
		l, r := f(lhs), f(rhs)
		switch {
		case l < r:
			return -1
		case l > r:
			return 1
		default:
			return 0
		}
	}
}

func toRow(vs ...string) Row {
	return Row{Columns: vs}
}

func ignoreError[T1, T2 any](v1 T1, _ T2) T1 {
	return v1
}

func sliceOf[V any](vs ...V) []V {
	return vs
}
