package stmtkind

import "fmt"

type StatementKind int

const (
	StatementKindInvalid StatementKind = iota
	StatementKindQuery
	StatementKindDDL
	StatementKindDML

	// StatementKindCall is a CALL statement. This statement is not other statement kind,
	// but it is a compatible with ExecuteSQL API..
	// https://cloud.google.com/spanner/docs/reference/standard-sql/procedural-language#call
	StatementKindCall

	// StatementKindGraph is a GRAPH statement. This statement is not other statement kind,
	// but it is a compatible with ExecuteSQL API..
	// https://cloud.google.com/spanner/docs/reference/standard-sql/graph-query-statementsl
	StatementKindGraph
)

func (k StatementKind) String() string {
	switch k {
	case StatementKindQuery:
		return "Query"
	case StatementKindDDL:
		return "DDL"
	case StatementKindDML:
		return "DML"
	case StatementKindCall:
		return "CALL"
	case StatementKindGraph:
		return "Graph"
	case StatementKindInvalid:
		return "Invalid"
	default:
		return fmt.Sprintf("UNKNOWN(%v)", int(k))
	}
}

func (k StatementKind) IsDDL() bool {
	return k == StatementKindDDL
}

func (k StatementKind) IsDML() bool {
	return k == StatementKindDML
}

func (k StatementKind) IsQuery() bool {
	return k == StatementKindQuery
}

func (k StatementKind) IsInvalid() bool {
	return k == StatementKindInvalid
}

func (k StatementKind) IsCall() bool {
	return k == StatementKindCall
}

func (k StatementKind) IsGraph() bool {
	return k == StatementKindGraph
}

// IsExecuteSQLCompatible is true when it is compatible with ExecuteSQL API family.
// Note: It is true if it is one of query, DML, Procedural(CALL), GRAPH statements,
// it means all statements except DDL statements.
func (k StatementKind) IsExecuteSQLCompatible() bool {
	return !k.IsInvalid() && !k.IsDDL()
}

// IsUpdateDDLCompatible is true when it is compatible with UpdateDatabaseDdl API and CreateDatabase API.
// Note: It is true only if it is a DDL statement.
func (k StatementKind) IsUpdateDDLCompatible() bool {
	return k.IsDDL()
}
