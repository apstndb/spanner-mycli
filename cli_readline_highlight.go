package main

import (
	"regexp"
	"slices"
	"strings"

	"github.com/nyaosorg/go-readline-ny"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/fatih/color"
)

// TokenRange represents a position range in the input
type TokenRange struct {
	Start int
	End   int
}

// HighlightRule represents a rule for applying highlight to specific tokens
type HighlightRule struct {
	Matcher  TokenMatcher
	Sequence string
}

// TokenMatcher is a function that determines if a token should be highlighted
type TokenMatcher func(tok token.Token) bool

// ClassifyToken determines the category of a token for highlighting purposes
func ClassifyToken(tok token.Token) string {
	switch {
	case isComment(tok):
		return "comment"
	case isStringLiteral(tok):
		return "string"
	case isNumericLiteral(tok):
		return "number"
	case isParameter(tok):
		return "parameter"
	case isKeyword(tok):
		return "keyword"
	case isIdentifier(tok):
		return "identifier"
	default:
		return "unknown"
	}
}

// isComment checks if a token has comments
func isComment(tok token.Token) bool {
	return len(tok.Comments) > 0
}

// isStringLiteral checks if a token is a string or bytes literal
func isStringLiteral(tok token.Token) bool {
	return tok.Kind == token.TokenString || tok.Kind == token.TokenBytes
}

// isNumericLiteral checks if a token is a numeric literal
func isNumericLiteral(tok token.Token) bool {
	return tok.Kind == token.TokenFloat || tok.Kind == token.TokenInt
}

// isParameter checks if a token is a parameter
func isParameter(tok token.Token) bool {
	return tok.Kind == token.TokenParam
}

// isKeyword checks if a token is a keyword
func isKeyword(tok token.Token) bool {
	return alnumRe.MatchString(string(tok.Kind))
}

// isIdentifier checks if a token is an identifier
func isIdentifier(tok token.Token) bool {
	return tok.Kind == token.TokenIdent
}

var alnumRe = regexp.MustCompile("^[a-zA-Z0-9]+$")

// GetTokenRanges extracts position ranges from a token
func GetTokenRanges(tok token.Token) []TokenRange {
	ranges := []TokenRange{{Start: int(tok.Pos), End: int(tok.End)}}
	
	// Add comment ranges
	for _, comment := range tok.Comments {
		ranges = append(ranges, TokenRange{Start: int(comment.Pos), End: int(comment.End)})
	}
	
	return ranges
}

// GetCommentRanges extracts only comment ranges from a token
func GetCommentRanges(tok token.Token) []TokenRange {
	var ranges []TokenRange
	for _, comment := range tok.Comments {
		ranges = append(ranges, TokenRange{Start: int(comment.Pos), End: int(comment.End)})
	}
	return ranges
}

// MatchToken checks if a token matches any of the given kinds
func MatchToken(tok token.Token, kinds ...token.TokenKind) bool {
	return slices.Contains(kinds, tok.Kind)
}

// CreateColorSequence generates ANSI color sequence from attributes
func CreateColorSequence(attrs ...color.Attribute) string {
	if color.NoColor {
		return ""
	}
	var sb strings.Builder
	color.New(attrs...).SetWriter(&sb)
	return sb.String()
}

// CreateHighlightRules creates default highlight rules
func CreateHighlightRules() []HighlightRule {
	return []HighlightRule{
		// Comments
		{
			Matcher:  isComment,
			Sequence: CreateColorSequence(color.FgWhite, color.Faint),
		},
		// String and byte literals
		{
			Matcher:  isStringLiteral,
			Sequence: CreateColorSequence(color.FgGreen, color.Bold),
		},
		// Numeric literals
		{
			Matcher:  isNumericLiteral,
			Sequence: CreateColorSequence(color.FgHiBlue, color.Bold),
		},
		// Parameters
		{
			Matcher:  isParameter,
			Sequence: CreateColorSequence(color.FgMagenta, color.Bold),
		},
		// Keywords
		{
			Matcher:  isKeyword,
			Sequence: CreateColorSequence(color.FgHiYellow, color.Bold),
		},
		// Identifiers
		{
			Matcher:  isIdentifier,
			Sequence: CreateColorSequence(color.FgHiWhite),
		},
	}
}

// ConvertRulesToReadlineHighlights converts highlight rules to readline.Highlight format
func ConvertRulesToReadlineHighlights(rules []HighlightRule) []readline.Highlight {
	highlights := make([]readline.Highlight, len(rules))
	for i, rule := range rules {
		highlights[i] = readline.Highlight{
			Pattern:  createHighlighterFunc(rule.Matcher),
			Sequence: rule.Sequence,
		}
	}
	return highlights
}

// createHighlighterFunc creates a highlighter function from a TokenMatcher
func createHighlighterFunc(matcher TokenMatcher) highlighterFunc {
	return tokenHighlighter(matcher)
}

// IsErrorUnclosedString checks if an error message indicates an unclosed string
func IsErrorUnclosedString(message string) bool {
	return message == errMessageUnclosedStringLiteral || message == errMessageUnclosedTripleQuotedStringLiteral
}

// IsErrorUnclosedComment checks if an error message indicates an unclosed comment
func IsErrorUnclosedComment(message string) bool {
	return message == errMessageUnclosedComment
}

// Error message constants
const (
	errMessageUnclosedTripleQuotedStringLiteral = `unclosed triple-quoted string literal`
	errMessageUnclosedStringLiteral             = `unclosed string literal`
	errMessageUnclosedComment                   = `unclosed comment`
)