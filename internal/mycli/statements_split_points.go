package mycli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/memebridge"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/char"
	"github.com/cloudspannerecosystem/memefish/token"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AddSplitPointsStatement struct {
	SplitPoints []*databasepb.SplitPoints
}

func (s *AddSplitPointsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	_, err := session.adminClient.AddSplitPoints(ctx, &databasepb.AddSplitPointsRequest{
		Database:    session.DatabasePath(),
		SplitPoints: s.SplitPoints,
		Initiator:   "spanner_mycli",
	})
	if err != nil {
		return nil, err
	}

	return &Result{}, nil
}

type ShowSplitPointsStatement struct{}

func (s *ShowSplitPointsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt := spanner.NewStatement("SELECT TABLE_NAME, INDEX_NAME, INITIATOR, SPLIT_KEY, EXPIRE_TIME FROM SPANNER_SYS.USER_SPLIT_POINTS")

	return executeInformationSchemaBasedStatementImpl(ctx, session,
		"SHOW SPLIT POINTS", stmt, true, nil)
}

func parseDropSplitPointsBody(body string) ([]*databasepb.SplitPoints, error) {
	p := newParser("", body)
	if err := p.NextToken(); err != nil {
		return nil, err
	}

	// Use the epoch timestamp to drop split points
	return parseSplitPoints(p, &timestamppb.Timestamp{Seconds: 0, Nanos: 0})
}

func newParser(filepath, s string) *memefish.Parser {
	return &memefish.Parser{
		Lexer: &memefish.Lexer{
			File: &token.File{
				Buffer:   s,
				FilePath: filepath,
			},
		},
	}
}

func parseAddSplitPointsBody(body string) ([]*databasepb.SplitPoints, error) {
	p := newParser("", body)
	if err := p.NextToken(); err != nil {
		return nil, err
	}

	expireTime, err := tryParseSplitPointsExpiredAt(p)
	if err != nil {
		return nil, err
	}

	return parseSplitPoints(p, expireTime)
}

func parseSplitPoints(p *memefish.Parser, expireTime *timestamppb.Timestamp) ([]*databasepb.SplitPoints, error) {
	var result []*databasepb.SplitPoints
	for p.Token.Kind != token.TokenEOF {
		sp, err := parseSplitPointsEntry(p, expireTime)
		if err != nil {
			return nil, err
		}
		result = append(result, sp)
	}
	return result, nil
}

func tryParseSplitPointsExpiredAt(p *memefish.Parser) (*timestamppb.Timestamp, error) {
	if !p.Token.IsKeywordLike("EXPIRED") {
		return nil, nil
	}

	if err := p.NextToken(); err != nil {
		return nil, err
	}

	if p.Token.Kind != "AT" {
		return nil, fmt.Errorf("expected AT, got %v", p.Token.Raw)
	}

	expr, err := parseExpr(p)
	if err != nil {
		return nil, err
	}

	gcv, err := memebridge.MemefishExprToGCV(expr)
	if err != nil {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339Nano, gcv.Value.GetStringValue())
	if err != nil {
		return nil, err
	}

	return timestamppb.New(t), nil
}

func parseSplitPointsEntry(p *memefish.Parser, expireTime *timestamppb.Timestamp) (*databasepb.SplitPoints, error) {
	var isIndex bool
	switch {
	case p.Token.IsKeywordLike("TABLE"):
	case p.Token.IsKeywordLike("INDEX"):
		isIndex = true
	default:
		return nil, fmt.Errorf("expected TABLE or INDEX, got %v", p.Token.Raw)
	}

	if err := p.NextToken(); err != nil {
		return nil, err
	}

	// the last ident is not consumed
	objectName, err := parseFQN(p)
	if err != nil {
		return nil, err
	}

	splitPointKey, err := parseSplitPointKey(p)
	if err != nil {
		return nil, err
	}

	if isIndex {
		splitPointKeys := sliceOf(splitPointKey)

		if p.Token.IsKeywordLike("TABLEKEY") {
			tableKey, err := parseSplitPointKey(p)
			if err != nil {
				return nil, err
			}

			splitPointKeys = append(splitPointKeys, tableKey)
		}

		return &databasepb.SplitPoints{
			Index:      objectName,
			Keys:       splitPointKeys,
			ExpireTime: expireTime,
		}, nil
	} else {
		return &databasepb.SplitPoints{
			Table:      objectName,
			Keys:       []*databasepb.SplitPoints_Key{splitPointKey},
			ExpireTime: expireTime,
		}, nil
	}
}

// parseIdentLikeWithOptDot process sequence of identifier with an optional dot.
// If the dot is appeared, both tokens are consumed. If not, no token is consumed.
// It is a workaround to use ParseExpr().
func parseIdentLikeWithOptDot(p *memefish.Parser) (s string, dot bool, err error) {
	lex := *p.Lexer

	s, err = peekIdentLike(p)
	if err != nil {
		return "", false, err
	}

	if err := p.NextToken(); err != nil {
		return "", false, err
	}

	if p.Token.Kind != "." {
		// restore the lexer state
		p.Lexer = &lex
		return s, false, nil
	}

	// consume the dot
	if err := p.NextToken(); err != nil {
		return "", false, err
	}

	return s, true, nil
}

func peekIdentLike(p *memefish.Parser) (s string, err error) {
	tok := p.Token
	switch {
	case tok.Kind == token.TokenIdent:
		return tok.AsString, nil
	case char.IsIdentStart(p.Token.Raw[0]):
		return tok.Raw, nil
	default:
		return "", fmt.Errorf("expected identifier, got %v", p.Token.Raw)
	}
}

func parseFQN(p *memefish.Parser) (string, error) {
	var idents []string
	for {
		s, dot, err := parseIdentLikeWithOptDot(p)
		if err != nil {
			return "", err
		}

		idents = append(idents, s)

		if !dot {
			break
		}
	}
	return strings.Join(idents, "."), nil
}

func parseSplitPointKey(p *memefish.Parser) (*databasepb.SplitPoints_Key, error) {
	expr, err := parseExpr(p)
	if err != nil {
		return nil, err
	}

	return parseLiteralExprForSplitPoint(expr)
}

// parseExpr parses the next expression.
// Precondition: The first token is skipped because of (*memefish.Parser).ParseExpr() behavior.
func parseExpr(p *memefish.Parser) (ast.Expr, error) {
	expr, err := p.ParseExpr()
	if err != nil {
		// ParseExpr() returns errors when there are remaining inputs after accepting a complete expression.
		// We must ignore these errors to continue processing.
		var me memefish.MultiError
		if errors.As(err, &me) {
			for _, e := range me {
				if !strings.Contains(e.Message, "expected token: <eof>") {
					return nil, fmt.Errorf("first error on parseExpr: %w", e)
				}
			}
			// skip
		} else {
			return nil, fmt.Errorf("parseExpr: %w", err)
		}
	}
	return expr, nil
}

func parseLiteralExprForSplitPoint(expr ast.Expr) (*databasepb.SplitPoints_Key, error) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		gcv, err := memebridge.MemefishExprToGCV(e)
		if err != nil {
			return nil, fmt.Errorf("expression is not a supported literal, expr: %v, err: %w", e.SQL(), err)
		}

		return &databasepb.SplitPoints_Key{KeyParts: &structpb.ListValue{Values: []*structpb.Value{gcv.Value}}}, nil
	case *ast.TupleStructLiteral:
		gcv, err := memebridge.MemefishExprToGCV(e)
		if err != nil {
			return nil, fmt.Errorf("expression is not a supported literal, expr: %v, err: %w", e.SQL(), err)
		}

		return &databasepb.SplitPoints_Key{KeyParts: gcv.Value.GetListValue()}, nil
	default:
		return nil, fmt.Errorf("unsupported expr as literals: %v", expr.SQL())
	}
}
