package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStripComments(t *testing.T) {
	for _, test := range []struct {
		desc  string
		input string
		want  string
	}{
		{desc: "no comment", input: "SELECT 1", want: "SELECT 1"},
		{desc: "line comment before EOF", input: "SELECT 1 // comment", want: "SELECT 1 "},
		{desc: "line comment", input: "SELECT 1// comment \n+ 2", want: "SELECT 1\n+ 2"},
		{desc: "inline multiline comment", input: `SELECT 1/**/+ 2`, want: "SELECT 1 + 2"},
		{desc: "multiline comment", input: "SELECT 1/*\n*/+ 2", want: "SELECT 1\n+ 2"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got, err := StripComments("", test.input)
			if err != nil {
				t.Errorf("StripComments() error = %v", err)
				return
			}
			if got != test.want {
				t.Errorf("StripComments() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSeparateInputPreserveCommentsWithStatus(t *testing.T) {
	const (
		terminatorHorizontal = `;`
		terminatorVertical   = `\G`
		terminatorUndefined  = ``
	)
	for _, tt := range []struct {
		desc       string
		input      string
		want       []RawStatement
		wantErr    error
		wantAnyErr bool
	}{
		{
			desc:  "closed double quoted",
			input: `SELECT "123"`,
			want: []RawStatement{
				{
					Statement:  `SELECT "123"`,
					End:        12,
					Terminator: terminatorUndefined,
				},
			},
		},
		{
			desc:  "non-closed double quoted",
			input: `SELECT "123`,
			want: []RawStatement{
				{
					Statement:  `SELECT "123`,
					End:        11,
					Terminator: terminatorUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  "non-closed single quoted",
			input: `SELECT '123`,
			want: []RawStatement{
				{
					Statement:  `SELECT '123`,
					End:        11,
					Terminator: terminatorUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  "closed single quoted",
			input: `SELECT '123'`,
			want: []RawStatement{
				{
					Statement:  `SELECT '123'`,
					End:        12,
					Terminator: terminatorUndefined,
				},
			},
		},
		{
			desc:  "non-closed back quoted",
			input: "SELECT `123",
			want: []RawStatement{
				{
					Statement:  "SELECT `123",
					End:        11,
					Terminator: terminatorUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  "closed back quoted",
			input: "SELECT `123`",
			want: []RawStatement{
				{
					Statement:  "SELECT `123`",
					End:        12,
					Terminator: terminatorUndefined,
				},
			},
		},
		{
			desc:  "closed comment",
			input: "SELECT /*123*/",
			want: []RawStatement{
				{
					Statement:  "SELECT /*123*/",
					End:        14,
					Terminator: terminatorUndefined,
				},
			},
		},
		{
			desc:  "closed comment",
			input: "SELECT /*123",
			want: []RawStatement{
				{
					Statement:  "SELECT /*123",
					End:        12,
					Terminator: terminatorUndefined,
				},
			},
			wantErr: &ErrLexerStatus{"*/"},
		},
		{
			desc:  "non-closed triple double quoted",
			input: `SELECT """123`,
			want: []RawStatement{
				{
					Statement:  `SELECT """123`,
					End:        13,
					Terminator: terminatorUndefined,
				},
			},
			wantErr: &ErrLexerStatus{`"""`},
		},
		{
			desc:  "closed triple double quoted",
			input: `SELECT """123"""`,
			want: []RawStatement{
				{
					Statement:  `SELECT """123"""`,
					End:        16,
					Terminator: terminatorUndefined,
				},
			},
		},
		{
			desc:  "non-closed triple single quoted",
			input: `SELECT '''123`,
			want: []RawStatement{
				{
					Statement:  `SELECT '''123`,
					End:        13,
					Terminator: terminatorUndefined,
				},
			},
			wantErr: &ErrLexerStatus{`'''`},
		},
		{
			desc:  "closed triple single quoted",
			input: `SELECT '''123'''`,
			want: []RawStatement{
				{
					Statement:  `SELECT '''123'''`,
					Pos:        0,
					End:        16,
					Terminator: terminatorUndefined,
				},
			},
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got, err := SeparateInputPreserveCommentsWithStatus("", tt.input)
			if (!tt.wantAnyErr && tt.wantErr == nil) && err != nil {
				t.Errorf("should success, but failed: %v", err)
			}
			if tt.wantAnyErr && err == nil {
				t.Error("should fail with any error, but success")
			}
			if diff := cmp.Diff(tt.wantErr, err); tt.wantErr != nil && diff != "" {
				t.Errorf("difference in err: (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(RawStatement{})); diff != "" {
				t.Errorf("difference in statements: (-want +got):\n%s", diff)
			}
		})
	}
}
