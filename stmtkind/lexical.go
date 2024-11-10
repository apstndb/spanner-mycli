package stmtkind

import (
	"fmt"

	"github.com/cloudspannerecosystem/memefish/token"

	"github.com/apstndb/spanner-mycli/internal"
)

var kindFirstTokensMap = map[StatementKind][]string{
	// Current prefixes of DDL statements
	// https://cloud.google.com/spanner/docs/reference/standard-sql/data-definition-language
	StatementKindDDL: {"CREATE", "ALTER", "DROP", "RENAME", "GRANT", "REVOKE", "ANALYZE"},
	StatementKindDML: {"INSERT", "DELETE", "UPDATE"},

	// It starts with "WITH" of CTE or query expression, it can be a ZetaSQL FROM query.
	// https://cloud.google.com/spanner/docs/reference/standard-sql/query-syntax#sql_syntax
	// https://github.com/google/zetasql/blob/master/docs/pipe-syntax.md#from-queries
	StatementKindQuery: {"SELECT", "WITH", "(", "FROM"},
	StatementKindGraph: {"GRAPH"},
	StatementKindCall:  {"CALL"},
}

// isKeywordLikeFuzzy is true when tok.IsKeywordLike(keywordLike) or tok.Kind == keywordLike
func isKeywordLikeFuzzy(tok token.Token, keywordLike string) bool {
	if tok.Kind == token.TokenIdent {
		return tok.IsKeywordLike(keywordLike)
	}
	return tok.Kind == token.TokenKind(keywordLike)
}

// oneOfKeywordLikeFuzzy is true when tok is a one of keywordLikes.
func oneOfKeywordLikeFuzzy(tok token.Token, keywordLikes ...string) bool {
	for _, k := range keywordLikes {
		if isKeywordLikeFuzzy(tok, k) {
			return true
		}
	}
	return false
}

func DetectLexical(s string) (StatementKind, error) {
	tok, err := internal.FirstNonHintToken("", s)
	if err != nil {
		return StatementKindInvalid, err
	}

	for kind, tokens := range kindFirstTokensMap {
		if oneOfKeywordLikeFuzzy(tok, tokens...) {
			return kind, nil
		}
	}

	return StatementKindInvalid, fmt.Errorf("unknown statement with first token: %v", tok.AsString)
}

func IsDDLLexical(s string) bool {
	return ignoreLast(DetectLexical(s)).IsDDL()
}

func IsDMLLexical(s string) bool {
	return ignoreLast(DetectLexical(s)).IsDML()
}

func IsQueryLexical(s string) bool {
	return ignoreLast(DetectLexical(s)).IsQuery()
}

func IsExecuteSQLCompatibleLexical(s string) bool {
	return ignoreLast(DetectLexical(s)).IsExecuteSQLCompatible()
}

func IsUpdateDDLCompatibleLexical(s string) bool {
	return ignoreLast(DetectLexical(s)).IsUpdateDDLCompatible()
}

func ignoreLast[T1, T2 any](v1 T1, _ T2) T1 {
	return v1
}
