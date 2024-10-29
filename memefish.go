package main

// This file will eventually become another library.

import (
	"fmt"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
	"strings"
)

type RawStatement struct {
	Pos, End   token.Pos
	Statement  string
	Terminator string
}

func (stmt *RawStatement) StripComments() (RawStatement, error) {
	result, err := StripComments("", stmt.Statement)

	// It can assume InputStatement.Statement doesn't have any terminating characters.
	return RawStatement{
		Statement:  result,
		Terminator: stmt.Terminator,
	}, err
}

type ErrLexerStatus struct {
	WaitingString string
}

func (e *ErrLexerStatus) Error() string {
	return fmt.Sprintf("lexer error with waiting: %v", e.WaitingString)
}

func SeparateInputPreserveCommentsWithStatus(filepath, s string) ([]RawStatement, error) {
	lex := newLexer(filepath, s)

	var results []RawStatement
	var pos token.Pos
outer:
	for {
		err := lex.NextToken()

		if err != nil {
			if e, ok := lo.ErrorsAs[*memefish.Error](err); ok {
				results = append(results, RawStatement{
					Pos:        pos,
					End:        e.Position.End,
					Statement:  lex.Buffer[pos:e.Position.End],
					Terminator: "",
				})

				switch e.Message {
				case `unclosed triple-quoted string literal`:
					if strings.HasPrefix(lex.Buffer[lex.Token.Pos:], `"""`) {
						return results, &ErrLexerStatus{
							WaitingString: `"""`,
						}
					}
					return results, &ErrLexerStatus{
						WaitingString: `'''`,
					}
				case `unclosed comment`:
					return results, &ErrLexerStatus{
						WaitingString: `*/`,
					}
				default:
					return results, err
				}
			}
			return results, err
		}

		// renew pos to first comment or first token of a statement.
		if pos.Invalid() {
			if len(lex.Token.Comments) > 0 {
				pos = lex.Token.Comments[0].Pos
			} else {
				pos = lex.Token.Pos
			}
		}

		switch lex.Token.Kind {
		case token.TokenEOF:
			// If pos:lex.Token.Pos is not empty, add remaining part of buffer to result.
			if pos != lex.Token.Pos {
				results = append(results, RawStatement{Statement: s[pos:lex.Token.Pos], Pos: pos, End: lex.Token.Pos, Terminator: ""})
			}

			// no need to continue
			break outer
		case ";":
			results = append(results, RawStatement{Statement: s[pos:lex.Token.Pos], Pos: pos, End: lex.Token.End, Terminator: ";"})

			pos = token.InvalidPos

			continue
		default:
		}
	}
	return results, nil
}

// StripComments strips comments in an input string without parsing.
// This function won't panic but return error if lexer become error state.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func StripComments(filepath, s string) (string, error) {
	// TODO: refactor
	lex := newLexer(filepath, s)

	var b strings.Builder
	var prevEnd token.Pos
	var stmtFirstPos token.Pos
	for {
		if lex.Token.Kind == ";" {
			stmtFirstPos = lex.Token.End
		}

		if len(lex.Token.Comments) > 0 {
			// flush all string before comments
			b.WriteString(s[prevEnd:lex.Token.Comments[0].Pos])
			if lex.Token.Kind == token.TokenEOF {
				// no need to continue
				break
			}

			var commentStrBuilder strings.Builder
			var hasNewline bool
			for _, comment := range lex.Token.Comments {
				// skip single line comment at the very first of statement.
				if stmtFirstPos == comment.Pos && comment.Space == "" && (strings.HasPrefix(comment.Raw, "--") || strings.HasPrefix(comment.Raw, "#")) {
					continue
				}
				commentStrBuilder.WriteString(comment.Space)
				if strings.ContainsAny(comment.Raw, "\n") {
					hasNewline = true
				}
			}
			commentStr := strings.TrimSpace(commentStrBuilder.String())

			if commentStr != "" {
				b.WriteString(commentStr)
			} else if stmtFirstPos != lex.Token.Comments[0].Pos {
				// Unless the comment is placed at the head of statement, comments will be a whitespace.
				if hasNewline {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}

			b.WriteString(lex.Token.Raw)
			prevEnd = lex.Token.End
		}

		// flush EOF
		if lex.Token.Kind == token.TokenEOF {
			b.WriteString(s[prevEnd:lex.Token.Pos])
			break
		}

		err := lex.NextToken()
		if err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func newLexer(filepath string, s string) *memefish.Lexer {
	lex := &memefish.Lexer{
		File: &token.File{
			FilePath: filepath,
			Buffer:   s,
		},
	}
	return lex
}
