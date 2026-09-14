// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/testdata/protos"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

type stringQuoteRow struct {
	NullCol      spanner.NullString   `spanner:"null_col"`
	StringNULL   string               `spanner:"string_null"`
	AngleCol     string               `spanner:"angle_col"`
	Ordinary     string               `spanner:"ordinary"`
	EmptyStr     string               `spanner:"empty_str"`
	JSONNullTok  string               `spanner:"json_null_tok"`
	QuotedNULL   string               `spanner:"quoted_null"`
	CommaStr     string               `spanner:"comma_str"`
	NullInt      spanner.NullInt64    `spanner:"null_int"`
	NullArr      []string             `spanner:"null_arr"`
	EmptyArr     []string             `spanner:"empty_arr"`
	Nested       []spanner.NullString `spanner:"nested"`
	NestedStruct stringQuoteStruct    `spanner:"nested_struct"`
	JSONNull     spanner.NullJSON     `spanner:"json_null"`
	SQLJSONNull  spanner.NullJSON     `spanner:"sql_json_null"`
}

type stringQuoteStruct struct {
	A spanner.NullString `spanner:"a"`
	B string             `spanner:"b"`
}

func stringQuoteItems() []stringQuoteRow {
	return []stringQuoteRow{{
		NullCol:     spanner.NullString{Valid: false},
		StringNULL:  "NULL",
		AngleCol:    "<NULL>",
		Ordinary:    "abc",
		EmptyStr:    "",
		JSONNullTok: "null",
		QuotedNULL:  `"NULL"`,
		CommaStr:    "a,b",
		NullInt:     spanner.NullInt64{Valid: false},
		NullArr:     nil,
		EmptyArr:    []string{},
		Nested:      []spanner.NullString{{Valid: false}, {StringVal: "NULL", Valid: true}, {StringVal: "abc", Valid: true}},
		NestedStruct: stringQuoteStruct{
			A: spanner.NullString{Valid: false},
			B: "NULL",
		},
		JSONNull:    spanner.NullJSON{Value: nil, Valid: true},
		SQLJSONNull: spanner.NullJSON{Valid: false},
	}}
}

func mustStringQuoteTyped(t *testing.T) (*sppb.ResultSetMetadata, []*spanner.Row, TableHeader) {
	t.Helper()
	enc := spancodec.MustNewRowEncoder[stringQuoteRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatalf("ResultSetMetadata: %v", err)
	}
	var rows []*spanner.Row
	for row, err := range enc.Rows(stringQuoteItems()) {
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		rows = append(rows, row)
	}
	return md, rows, toTableHeader(md.GetRowType().GetFields())
}

func stringQuoteSysVars(mode enums.DisplayMode, quote enums.StringQuoteMode, styled enums.StyledMode) systemVariables {
	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = mode
	sv.Display.StringQuoteMode = quote
	sv.Display.StyledOutput = styled
	sv.Display.SQLTableName = "Items"
	return sv
}

func printStringQuote(t *testing.T, sv *systemVariables, header TableHeader, md *sppb.ResultSetMetadata, rows []*spanner.Row) string {
	t.Helper()
	var buf bytes.Buffer
	if err := printTableData(sv, 0, &buf, &Result{
		TableHeader: header,
		Body:        TypedBody(&TypedRows{Metadata: md, Rows: rows}),
	}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestNeedsAutoStringQuote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    string
		want bool
	}{
		{"", true},
		{" ", true},
		{" NULL", true},
		{"NULL ", true},
		{"NULL", true},
		{"null", true},
		{"true", true},
		{"false", true},
		{"NaN", true},
		{"+Inf", true},
		{"-Inf", true},
		{"Null", false},
		{"TRUE", false},
		{"abc", false},
		{"<NULL>", false},
		{"123", true},
		{"0123", true},
		{"+1", true},
		{"1e3", true},
		{".5", true},
		{"1.", true},
		{"1.5", true},
		{"0xFF", false},
		{"1+1", false},
		{`"NULL"`, true},
		{`\`, true},
		{"a,b", true},
		{"[1]", true},
		{"{x}", true},
		{"a]b", true},
		{"\n", true},
		{"\x00", true},
		{"こんにちは", false},
		{"'NULL'", false},
	}
	for _, tc := range cases {
		if got := needsAutoStringQuote(tc.s); got != tc.want {
			t.Errorf("needsAutoStringQuote(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestApplyStringQuoteDisplayPreservesInput(t *testing.T) {
	t.Parallel()
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeNone, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != fc {
		t.Fatal("NONE must return the same config pointer")
	}
	for _, mode := range []enums.DisplayMode{
		enums.DisplayModeCSV, enums.DisplayModeTSV, enums.DisplayModeJSONL,
		enums.DisplayModeSQLInsert, enums.DisplayModeTab, enums.DisplayModeHTML, enums.DisplayModeXML,
	} {
		got, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAlways, mode)
		if err != nil {
			t.Fatal(err)
		}
		if got != fc {
			t.Fatalf("excluded mode %s changed the display config", mode)
		}
	}
	updated, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAuto, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}
	if updated == fc {
		t.Fatal("AUTO must clone")
	}
	if fc.GetNullString() != "NULL" || updated.GetNullString() != "NULL" {
		t.Fatal("helper changed NullString")
	}
}

func TestCLIStringQuoteModeNONEIdentityAndAUTOContract(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustStringQuoteTyped(t)

	none := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeNone, enums.StyledModeFalse)
	auto := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	always := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAlways, enums.StyledModeFalse)
	defaultNone := newSystemVariablesWithDefaults()
	defaultNone.Display.StyledOutput = enums.StyledModeFalse

	gotNone := printStringQuote(t, &none, header, md, rawRows)
	gotDefault := printStringQuote(t, &defaultNone, header, md, rawRows)
	if diff := cmp.Diff(gotNone, gotDefault); diff != "" {
		t.Fatalf("explicit NONE must match default unset (-explicit +default):\n%s", diff)
	}
	if strings.Contains(gotNone, "\033[") {
		t.Fatalf("colors-off TABLE leaked ANSI:\n%s", gotNone)
	}

	cellsNone, err := deriveDisplayRows(&none, &TypedRows{Metadata: md, Rows: rawRows})
	if err != nil {
		t.Fatal(err)
	}
	cellsAuto, err := deriveDisplayRows(&auto, &TypedRows{Metadata: md, Rows: rawRows})
	if err != nil {
		t.Fatal(err)
	}
	cellsAlways, err := deriveDisplayRows(&always, &TypedRows{Metadata: md, Rows: rawRows})
	if err != nil {
		t.Fatal(err)
	}

	// 0 null, 1 STRING NULL, 2 <NULL>, 3 abc, 4 empty, 5 null tok, 6 "NULL", 7 a,b
	wantNone := []string{"NULL", "NULL", "<NULL>", "abc", "", "null", `"NULL"`, "a,b"}
	wantAuto := []string{"NULL", `"NULL"`, "<NULL>", "abc", `""`, `"null"`, strconv.Quote(`"NULL"`), strconv.Quote("a,b")}
	wantAlways := []string{"NULL", `"NULL"`, `"<NULL>"`, `"abc"`, `""`, `"null"`, strconv.Quote(`"NULL"`), strconv.Quote("a,b")}
	for i, want := range wantNone {
		if got := cellsNone[0][i].RawText(); got != want {
			t.Errorf("NONE cell %d = %q, want %q", i, got, want)
		}
	}
	for i, want := range wantAuto {
		if got := cellsAuto[0][i].RawText(); got != want {
			t.Errorf("AUTO cell %d = %q, want %q", i, got, want)
		}
	}
	for i, want := range wantAlways {
		if got := cellsAlways[0][i].RawText(); got != want {
			t.Errorf("ALWAYS cell %d = %q, want %q", i, got, want)
		}
	}
	if cellsAuto[0][11].RawText() != `[NULL, "NULL", abc]` {
		t.Errorf("AUTO nested ARRAY = %q", cellsAuto[0][11].RawText())
	}
	if cellsAlways[0][11].RawText() != `[NULL, "NULL", "abc"]` {
		t.Errorf("ALWAYS nested ARRAY = %q", cellsAlways[0][11].RawText())
	}
	if cellsAuto[0][12].RawText() != `[NULL, "NULL"]` {
		t.Errorf("AUTO STRUCT = %q", cellsAuto[0][12].RawText())
	}
	if cellsAuto[0][13].RawText() != "null" {
		t.Errorf("JSON JSON-null = %q", cellsAuto[0][13].RawText())
	}
	if cellsAuto[0][14].RawText() != "NULL" {
		t.Errorf("SQL-null JSON = %q", cellsAuto[0][14].RawText())
	}
	if _, ok := cellsAuto[0][0].(format.NoWrapCell); !ok {
		t.Fatalf("AUTO SQL NULL cell type %T, want NoWrapCell", cellsAuto[0][0])
	}
	if _, ok := cellsAuto[0][1].(format.NoWrapCell); ok {
		t.Fatal("quoted STRING NULL must not become NoWrapCell")
	}

	styledAuto := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAuto, enums.StyledModeTrue)
	styledOut := printStringQuote(t, &styledAuto, header, md, rawRows)
	if !strings.Contains(styledOut, "\033[2mNULL\033[0m") {
		t.Fatalf("styled SQL NULL should keep dim around NULL:\n%s", styledOut)
	}
	if strings.Contains(styledOut, "\033[2m\"NULL\"") {
		t.Fatalf("quoted STRING NULL must not use NULL dim styling:\n%s", styledOut)
	}

	vertAuto := stringQuoteSysVars(enums.DisplayModeVertical, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	gotVert := printStringQuote(t, &vertAuto, header, md, rawRows)
	if !strings.Contains(gotVert, "null_col: NULL") || !strings.Contains(gotVert, `string_null: "NULL"`) || !strings.Contains(gotVert, "ordinary: abc") || !strings.Contains(gotVert, `empty_str: ""`) {
		t.Fatalf("VERTICAL AUTO contract mismatch:\n%s", gotVert)
	}
}

func TestCLIStringQuoteModeArrayCommaVsTwoElements(t *testing.T) {
	t.Parallel()
	type oneComma struct {
		Arr []string `spanner:"arr"`
	}
	type twoElem struct {
		Arr []string `spanner:"arr"`
	}
	encOne := spancodec.MustNewRowEncoder[oneComma]()
	mdOne, err := encOne.ResultSetMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var oneRows []*spanner.Row
	for row, err := range encOne.Rows([]oneComma{{Arr: []string{"a,b"}}}) {
		if err != nil {
			t.Fatal(err)
		}
		oneRows = append(oneRows, row)
	}
	encTwo := spancodec.MustNewRowEncoder[twoElem]()
	mdTwo, err := encTwo.ResultSetMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var twoRows []*spanner.Row
	for row, err := range encTwo.Rows([]twoElem{{Arr: []string{"a", "b"}}}) {
		if err != nil {
			t.Fatal(err)
		}
		twoRows = append(twoRows, row)
	}

	sv := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	oneCells, err := deriveDisplayRows(&sv, &TypedRows{Metadata: mdOne, Rows: oneRows})
	if err != nil {
		t.Fatal(err)
	}
	twoCells, err := deriveDisplayRows(&sv, &TypedRows{Metadata: mdTwo, Rows: twoRows})
	if err != nil {
		t.Fatal(err)
	}
	gotOne := oneCells[0][0].RawText()
	gotTwo := twoCells[0][0].RawText()
	if gotOne != `["a,b"]` {
		t.Fatalf("one-element ARRAY of a,b = %q, want [\"a,b\"]", gotOne)
	}
	if gotTwo != `[a, b]` {
		t.Fatalf("two-element ARRAY a,b = %q, want [a, b]", gotTwo)
	}
	if gotOne == gotTwo {
		t.Fatal("AUTO must keep one-element a,b distinct from two elements a and b")
	}
}

func TestCLIStringQuoteModeAUTONumberAndTokenBoundaries(t *testing.T) {
	t.Parallel()
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	auto, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAuto, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}
	always, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAlways, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		val       spanner.GenericColumnValue
		auto, all string
	}{
		{"sql null", gcvctor.NullFromCode(sppb.TypeCode_STRING), "NULL", "NULL"},
		{"empty", gcvctor.StringValue(""), `""`, `""`},
		{"abc", gcvctor.StringValue("abc"), "abc", `"abc"`},
		{"angle", gcvctor.StringValue("<NULL>"), "<NULL>", `"<NULL>"`},
		{"NULL", gcvctor.StringValue("NULL"), `"NULL"`, `"NULL"`},
		{"null", gcvctor.StringValue("null"), `"null"`, `"null"`},
		{"true", gcvctor.StringValue("true"), `"true"`, `"true"`},
		{"TRUE", gcvctor.StringValue("TRUE"), "TRUE", `"TRUE"`},
		{"NaN", gcvctor.StringValue("NaN"), `"NaN"`, `"NaN"`},
		{"+Inf", gcvctor.StringValue("+Inf"), `"+Inf"`, `"+Inf"`},
		{"-Inf", gcvctor.StringValue("-Inf"), `"-Inf"`, `"-Inf"`},
		{".5", gcvctor.StringValue(".5"), `".5"`, `".5"`},
		{"1.", gcvctor.StringValue("1."), `"1."`, `"1."`},
		{"0xFF", gcvctor.StringValue("0xFF"), "0xFF", `"0xFF"`},
		{"1+1", gcvctor.StringValue("1+1"), "1+1", `"1+1"`},
		{"quoted", gcvctor.StringValue(`"NULL"`), strconv.Quote(`"NULL"`), strconv.Quote(`"NULL"`)},
		{"already", gcvctor.StringValue(strconv.Quote(`"NULL"`)), strconv.Quote(strconv.Quote(`"NULL"`)), strconv.Quote(strconv.Quote(`"NULL"`))},
		{"nl", gcvctor.StringValue("\n"), strconv.Quote("\n"), strconv.Quote("\n")},
		{"nul", gcvctor.StringValue("\x00"), strconv.Quote("\x00"), strconv.Quote("\x00")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotAuto, err := auto.FormatToplevelColumn(tc.val)
			if err != nil {
				t.Fatal(err)
			}
			gotAll, err := always.FormatToplevelColumn(tc.val)
			if err != nil {
				t.Fatal(err)
			}
			if gotAuto != tc.auto || gotAll != tc.all {
				t.Fatalf("auto=%q want %q; always=%q want %q", gotAuto, tc.auto, gotAll, tc.all)
			}
		})
	}
}

func TestCLIStringQuoteModeExcludedFormatsUnchanged(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustStringQuoteTyped(t)
	for _, mode := range []enums.DisplayMode{
		enums.DisplayModeCSV, enums.DisplayModeTSV, enums.DisplayModeJSONL,
		enums.DisplayModeSQLInsert, enums.DisplayModeTab, enums.DisplayModeHTML, enums.DisplayModeXML,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			none := stringQuoteSysVars(mode, enums.StringQuoteModeNone, enums.StyledModeFalse)
			auto := stringQuoteSysVars(mode, enums.StringQuoteModeAuto, enums.StyledModeFalse)
			always := stringQuoteSysVars(mode, enums.StringQuoteModeAlways, enums.StyledModeFalse)
			gotNone := printStringQuote(t, &none, header, md, rawRows)
			gotAuto := printStringQuote(t, &auto, header, md, rawRows)
			gotAlways := printStringQuote(t, &always, header, md, rawRows)
			if diff := cmp.Diff(gotNone, gotAuto); diff != "" {
				t.Fatalf("AUTO must not change %s:\n%s", mode, diff)
			}
			if diff := cmp.Diff(gotNone, gotAlways); diff != "" {
				t.Fatalf("ALWAYS must not change %s:\n%s", mode, diff)
			}
		})
	}
}

func TestCLIStringQuoteModeSQLExportFallbackKeepsExistingBytes(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustStringQuoteTyped(t)
	tableNone := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeNone, enums.StyledModeFalse)
	want := printStringQuote(t, &tableNone, header, md, rawRows)

	sqlAuto := stringQuoteSysVars(enums.DisplayModeSQLInsert, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	var buf bytes.Buffer
	if err := printTableData(&sqlAuto, 0, &buf, &Result{
		TableHeader:      header,
		SQLExportAllowed: false,
		Body: TypedBody(&TypedRows{
			Metadata:         md,
			Rows:             rawRows,
			SQLExportAllowed: false,
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Fatalf("SQL-export fallback with AUTO must keep default TABLE bytes:\n%s", diff)
	}
}

func TestCLIStringQuoteModeThenReturnUsesTypedPolicy(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustStringQuoteTyped(t)
	sv := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	got, err := renderDMLReturnedRows(&sv, header, md, rawRows)
	if err != nil {
		t.Fatal(err)
	}
	want := printStringQuote(t, &sv, header, md, rawRows)
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Fatalf("THEN RETURN must match typed TABLE:\n%s", diff)
	}
}

func TestCLIStringQuoteModePresentationRowsUnchanged(t *testing.T) {
	t.Parallel()
	sv := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAlways, enums.StyledModeFalse)
	result := &Result{
		TableHeader: toTableHeader("plan"),
		Body:        PresentationBody([]Row{format.StringsToRow("NULL")}),
	}
	var buf bytes.Buffer
	if err := printTableData(&sv, 0, &buf, result); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"NULL"`) {
		t.Fatalf("preformatted presentation NULL must not be quote-wrapped:\n%s", buf.String())
	}
}

func TestCLIStringQuoteModePreservesProtoEnumPlugins(t *testing.T) {
	t.Parallel()
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(protos.File_singer_proto)},
	}
	base, err := decoder.FormatConfigWithProto(fds, false)
	if err != nil {
		t.Fatal(err)
	}
	seamed, err := applyStringQuoteDisplay(base, enums.StringQuoteModeAlways, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}

	enumVal := gcvctor.EnumValue("examples.spanner.music.Genre", int64(protos.Genre_POP))
	protoVal := gcvctor.ProtoValue("examples.spanner.music.SingerInfo", mustMarshalSingerQuote(t))
	nullProto := gcvctor.NullOf(protoVal.Type)

	baseEnum, err := base.FormatToplevelColumn(enumVal)
	if err != nil {
		t.Fatal(err)
	}
	seamEnum, err := seamed.FormatToplevelColumn(enumVal)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(baseEnum, seamEnum); diff != "" {
		t.Fatalf("enum formatting changed:\n%s", diff)
	}
	baseProto, err := base.FormatToplevelColumn(protoVal)
	if err != nil {
		t.Fatal(err)
	}
	seamProto, err := seamed.FormatToplevelColumn(protoVal)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(baseProto, seamProto); diff != "" {
		t.Fatalf("proto formatting changed:\n%s", diff)
	}
	baseNull, err := base.FormatToplevelColumn(nullProto)
	if err != nil {
		t.Fatal(err)
	}
	seamNull, err := seamed.FormatToplevelColumn(nullProto)
	if err != nil {
		t.Fatal(err)
	}
	if baseNull != "NULL" || seamNull != "NULL" {
		t.Fatalf("null proto: base=%q seamed=%q", baseNull, seamNull)
	}
}

func TestCLIStringQuoteModeSetShowResetLocal(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	got, err := sv.Get("CLI_STRING_QUOTE_MODE")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("CLI_STRING_QUOTE_MODE", "NONE"), got); diff != "" {
		t.Fatalf("default SHOW mismatch:\n%s", diff)
	}
	if err := sv.SetFromSimple("CLI_STRING_QUOTE_MODE", "AUTO"); err != nil {
		t.Fatal(err)
	}
	if sv.Display.StringQuoteMode != enums.StringQuoteModeAuto {
		t.Fatal("SET AUTO did not stick")
	}
	before := sv.Display.StringQuoteMode
	if err := sv.SetFromSimple("CLI_STRING_QUOTE_MODE", "NOPE"); err == nil {
		t.Fatal("expected invalid ENUM error")
	}
	if sv.Display.StringQuoteMode != before {
		t.Fatal("rejected SET must preserve state")
	}
	if err := sv.SetFromSimple("CLI_STRING_QUOTE_MODE", "ALWAYS"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("CLI_STRING_QUOTE_MODE"); err != nil {
		t.Fatal(err)
	}
	if sv.Display.StringQuoteMode != enums.StringQuoteModeNone {
		t.Fatal("RESET did not restore NONE")
	}

	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_STRING_QUOTE_MODE", Value: "ALWAYS"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_STRING_QUOTE_MODE"); got != "ALWAYS" {
		t.Fatalf("SET LOCAL = %s", got)
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_STRING_QUOTE_MODE"); got != "NONE" {
		t.Fatalf("LOCAL should restore after COMMIT: %s", got)
	}
}

func TestPrepareAndClientSideFormatContextHonorStringQuoteMode(t *testing.T) {
	t.Parallel()
	sv := stringQuoteSysVars(enums.DisplayModeTable, enums.StringQuoteModeAuto, enums.StyledModeFalse)
	render, err := prepareFormatConfig("SELECT 1", &sv, queryRenderingFrom(&sv))
	if err != nil {
		t.Fatal(err)
	}
	lit, err := render.Spanvalue.FormatToplevelColumn(gcvctor.StringValue("NULL"))
	if err != nil {
		t.Fatal(err)
	}
	if lit != `"NULL"` {
		t.Fatalf("prepareFormatConfig TABLE STRING NULL = %q", lit)
	}
	ordinary, err := render.Spanvalue.FormatToplevelColumn(gcvctor.StringValue("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if ordinary != "abc" {
		t.Fatalf("prepareFormatConfig TABLE abc = %q", ordinary)
	}

	csv := stringQuoteSysVars(enums.DisplayModeCSV, enums.StringQuoteModeAlways, enums.StyledModeFalse)
	csvRender, err := prepareFormatConfig("SELECT 1", &csv, queryRenderingFrom(&csv))
	if err != nil {
		t.Fatal(err)
	}
	csvLit, err := csvRender.Spanvalue.FormatToplevelColumn(gcvctor.StringValue("NULL"))
	if err != nil {
		t.Fatal(err)
	}
	if csvLit != "NULL" {
		t.Fatalf("prepareFormatConfig CSV STRING NULL = %q", csvLit)
	}

	fc, _, err := clientSideFormatContext(&sv)
	if err != nil {
		t.Fatal(err)
	}
	clientLit, err := fc.FormatToplevelColumn(gcvctor.StringValue("NULL"))
	if err != nil {
		t.Fatal(err)
	}
	if clientLit != `"NULL"` {
		t.Fatalf("clientSideFormatContext TABLE STRING NULL = %q", clientLit)
	}
}

func mustMarshalSingerQuote(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCLIStringQuoteModeHelperRejectsEmptyNullString(t *testing.T) {
	t.Parallel()
	fc := spanvalue.SpannerCLICompatibleFormatConfig().Clone()
	fc.NullString = ""
	if _, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAlways, enums.DisplayModeTable); err == nil {
		t.Fatal("expected applyStringQuoteDisplay error for empty NullString")
	}
	if _, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeAuto, enums.DisplayModeTable); err == nil {
		t.Fatal("expected applyStringQuoteDisplay AUTO error for empty NullString")
	}
	none, err := applyStringQuoteDisplay(fc, enums.StringQuoteModeNone, enums.DisplayModeTable)
	if err != nil {
		t.Fatalf("NONE must skip Validate: %v", err)
	}
	if none != fc {
		t.Fatal("NONE must return the input pointer")
	}
}
