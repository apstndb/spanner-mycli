package internal

// This file will eventually become another library.

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
	"spheric.cloud/xiter"
)

type RawStatement struct {
	Pos, End   token.Pos
	Statement  string
	Terminator string
}

// lexerSeq transfer memefish.Lexer to iter.Seq2 with error.
// If it reaches to EOF, it stops without error.
func lexerSeq(lexer *memefish.Lexer) iter.Seq2[token.Token, error] {
	return func(yield func(token.Token, error) bool) {
		for {
			if err := lexer.NextToken(); err != nil {
				_ = yield(lexer.Token, err)
				return
			}

			if lexer.Token.Kind == token.TokenEOF {
				_ = yield(lexer.Token, nil)
				return
			}

			if !yield(lexer.Token, nil) {
				return
			}
		}
	}
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
	lexer := newLexer(filepath, s)

	var results []RawStatement
	var pos token.Pos
outer:
	for tok, err := range lexerSeq(lexer) {
		if err != nil {
			if err, ok := lo.ErrorsAs[*memefish.Error](err); ok {
				results = append(results, RawStatement{Pos: pos, End: err.Position.End, Statement: lexer.Buffer[pos:err.Position.End]})
				return results, toErrLexerStatus(err, lexer.Buffer[tok.Pos:])
			}
			return results, err
		}

		// renew pos to first comment or first token of a statement.
		if pos.Invalid() {
			tokenComment, ok := lo.First(tok.Comments)
			pos = lo.Ternary(ok, tokenComment.Pos, tok.Pos)
		}

		switch tok.Kind {
		case token.TokenEOF:
			// If pos:tok.Pos is not empty, add remaining part of buffer to result.
			if pos != tok.Pos {
				results = append(results, RawStatement{Statement: s[pos:tok.Pos], Pos: pos, End: tok.Pos})
			}
			// no need to continue
			break outer
		case ";":
			results = append(results, RawStatement{Statement: s[pos:tok.Pos], Pos: pos, End: tok.End, Terminator: ";"})
			pos = token.InvalidPos
		default:
		}
	}
	return results, nil
}

const errMessageUnclosedTripleQuotedStringLiteral = `unclosed triple-quoted string literal`
const errMessageUnclosedComment = `unclosed comment`

// NOTE: memefish.Error.Message can be changed.
func toErrLexerStatus(err *memefish.Error, head string) error {
	switch {
	case err.Message == errMessageUnclosedTripleQuotedStringLiteral && strings.HasPrefix(head, `"""`):
		return &ErrLexerStatus{WaitingString: `"""`}
	case err.Message == errMessageUnclosedTripleQuotedStringLiteral:
		return &ErrLexerStatus{WaitingString: `'''`}
	case err.Message == errMessageUnclosedComment:
		return &ErrLexerStatus{WaitingString: `*/`}
	default:
		return err
	}
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
	for tok, err := range lexerSeq(lex) {
		if err != nil {
			return "", err
		}

		if tok.Kind == ";" {
			stmtFirstPos = tok.End
		}

		if comment, ok := lo.First(tok.Comments); ok {
			// flush all string before comments
			b.WriteString(s[prevEnd:comment.Pos])
			if tok.Kind == token.TokenEOF {
				// no need to continue
				break
			}

			/// var spacesBuilder strings.Builder
			it := xiter.Filter(slices.Values(tok.Comments), func(comment token.TokenComment) bool {
				raw := comment.Raw
				switch {
				case stmtFirstPos != comment.Pos:
					return true
				case comment.Space != "":
					return true
				case strings.HasPrefix(raw, "--") || strings.HasPrefix(raw, "#"):
					return false
				default:
					return false
				}
			})

			hasNewline := xiter.Any(it, func(comment token.TokenComment) bool {
				return strings.ContainsAny(comment.Raw, "\n")
			})
			if stmtFirstPos != comment.Pos {
				// Unless the comment is placed at the head of statement, comments will be a whitespace.
				if hasNewline {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}

			b.WriteString(tok.Raw)
			prevEnd = tok.End
		}

		// flush EOF
		if tok.Kind == token.TokenEOF {
			b.WriteString(s[prevEnd:tok.Pos])
			break
		}

	}
	return b.String(), nil
}

var errNoValidToken = errors.New("no valid token before EOF")

// FirstNonHintToken returns the first non-hint token.
// If there is no valid token before EOF, it returns error.
// filepath can be empty, it is only used in error message.
func FirstNonHintToken(filepath, s string) (token.Token, error) {
	var inHint bool
	for tok, err := range lexerSeq(newLexer(filepath, s)) {
		switch {
		case err != nil:
			return tok, err
		case tok.Kind == token.TokenEOF:
			return tok, errNoValidToken
		case tok.Kind == "@":
			inHint = true
			continue
		case inHint && tok.Kind == "}":
			inHint = false
			continue
		case inHint:
			continue
		default:
			return tok, nil
		}
	}

	panic("unreachable end of FirstNonHintToken")
}

// SimpleSkipHints strips hints in an input string without parsing.
// It don't preserve any comments and whitespaces. All tokens are separated with a single whitespace.
// filepath can be empty, it is only used in error message.
func SimpleSkipHints(filepath, s string) (string, error) {
	lex := newLexer(filepath, s)

	var b strings.Builder
	var inHint bool
loop:
	for tok, err := range lexerSeq(lex) {
		switch {
		case err != nil:
			return "", err
		case tok.Kind == token.TokenEOF:
			break loop
		case tok.Kind == "@":
			inHint = true
			continue
		case inHint && tok.Kind == "}":
			inHint = false
			continue
		case inHint:
			continue
		default:
			if b.Len() > 0 {
				b.WriteRune(' ')
			}
			b.WriteString(tok.Raw)
		}
	}
	return b.String(), nil
}

// SimpleStripComments strips comments in an input string without parsing.
// It don't preserve whitespaces. All tokens are separated with a single whitespace.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func SimpleStripComments(filepath, s string) (string, error) {
	lex := newLexer(filepath, s)

	var b strings.Builder
	for tok, err := range lexerSeq(lex) {
		if err != nil {
			return "", err
		}

		if tok.Kind == token.TokenEOF {
			break
		}

		if b.Len() > 0 {
			b.WriteRune(' ')
		}

		b.WriteString(tok.Raw)
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
