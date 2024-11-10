package stmtkind

import (
	"github.com/cloudspannerecosystem/memefish/ast"
)

func DetectSemantic(n ast.Statement) StatementKind {
	switch n.(type) {
	case ast.DML:
		return StatementKindDML
	case ast.DDL:
		return StatementKindDDL
	case *ast.QueryStatement:
		return StatementKindQuery
	default:
		return StatementKindInvalid
	}
}

func IsDMLSemantic(stmt ast.Statement) bool {
	return DetectSemantic(stmt).IsDML()
}

func IsQuerySemantic(stmt ast.Statement) bool {
	return DetectSemantic(stmt).IsQuery()
}

func IsDDLSemantic(stmt ast.Statement) bool {
	return DetectSemantic(stmt).IsDDL()
}

func IsExecuteSQLCompatibleSemantic(stmt ast.Statement) bool {
	return DetectSemantic(stmt).IsExecuteSQLCompatible()
}

func IsUpdateDDLCompatibleSemantic(stmt ast.Statement) bool {
	return DetectSemantic(stmt).IsUpdateDDLCompatible()
}
