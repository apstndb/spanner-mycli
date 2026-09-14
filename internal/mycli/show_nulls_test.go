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

type showNullsRow struct {
	NullCol      spanner.NullString   `spanner:"null_col"`
	StringCol    string               `spanner:"string_col"`
	AngleCol     string               `spanner:"angle_col"`
	QuotedNULL   string               `spanner:"quoted_null"`
	EmptyStr     string               `spanner:"empty_str"`
	NullInt      spanner.NullInt64    `spanner:"null_int"`
	NullBytes    []byte               `spanner:"null_bytes"`
	EmptyBytes   []byte               `spanner:"empty_bytes"`
	NullArr      []string             `spanner:"null_arr"`
	EmptyArr     []string             `spanner:"empty_arr"`
	Nested       []spanner.NullString `spanner:"nested"`
	NestedStruct showNullsStruct      `spanner:"nested_struct"`
	JSONNull     spanner.NullJSON     `spanner:"json_null"`
	SQLJSONNull  spanner.NullJSON     `spanner:"sql_json_null"`
	Ordinary     string               `spanner:"ordinary"`
	EscapedIn    string               `spanner:"escaped_in"`
}

type showNullsStruct struct {
	A spanner.NullString `spanner:"a"`
	B string             `spanner:"b"`
}

func showNullsItems() []showNullsRow {
	return []showNullsRow{{
		NullCol:    spanner.NullString{Valid: false},
		StringCol:  showNullsLiteral,
		AngleCol:   "<NULL>",
		QuotedNULL: `"NULL"`,
		EmptyStr:   "",
		NullInt:    spanner.NullInt64{Valid: false},
		NullBytes:  nil,
		EmptyBytes: []byte{},
		NullArr:    nil,
		EmptyArr:   []string{},
		Nested:     []spanner.NullString{{Valid: false}, {StringVal: "NULL", Valid: true}, {StringVal: `"NULL"`, Valid: true}},
		NestedStruct: showNullsStruct{
			A: spanner.NullString{Valid: false},
			B: "NULL",
		},
		JSONNull:    spanner.NullJSON{Value: nil, Valid: true},
		SQLJSONNull: spanner.NullJSON{Valid: false},
		Ordinary:    "abc",
		EscapedIn:   strconv.Quote(`"NULL"`),
	}}
}

func mustShowNullsTyped(t *testing.T) (*sppb.ResultSetMetadata, []*spanner.Row, TableHeader) {
	t.Helper()
	enc := spancodec.MustNewRowEncoder[showNullsRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatalf("ResultSetMetadata: %v", err)
	}
	var rows []*spanner.Row
	for row, err := range enc.Rows(showNullsItems()) {
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		rows = append(rows, row)
	}
	return md, rows, toTableHeader(md.GetRowType().GetFields())
}

func showNullsSysVars(mode enums.DisplayMode, showNulls bool, styled enums.StyledMode) systemVariables {
	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = mode
	sv.Display.ShowNulls = showNulls
	sv.Display.StyledOutput = styled
	sv.Display.SQLTableName = "Items"
	return sv
}

func printShowNulls(t *testing.T, sv *systemVariables, header TableHeader, md *sppb.ResultSetMetadata, rows []*spanner.Row) string {
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

func TestApplyShowNullsDisplayPreservesInput(t *testing.T) {
	t.Parallel()
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if fc.GetNullString() != "NULL" {
		t.Fatalf("preset NullString = %q", fc.GetNullString())
	}

	unchanged, err := applyShowNullsDisplay(fc, false, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != fc {
		t.Fatal("disabled helper must return the same config pointer")
	}
	if fc.GetNullString() != "NULL" {
		t.Fatal("disabled helper mutated the input")
	}

	for _, mode := range []enums.DisplayMode{
		enums.DisplayModeCSV, enums.DisplayModeTSV, enums.DisplayModeJSONL,
		enums.DisplayModeSQLInsert, enums.DisplayModeTab, enums.DisplayModeHTML, enums.DisplayModeXML,
	} {
		got, err := applyShowNullsDisplay(fc, true, mode)
		if err != nil {
			t.Fatal(err)
		}
		if got != fc || fc.GetNullString() != "NULL" {
			t.Fatalf("excluded mode %s changed the display config", mode)
		}
	}

	updated, err := applyShowNullsDisplay(fc, true, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}
	if updated == fc {
		t.Fatal("enabled helper must clone")
	}
	if fc.GetNullString() != "NULL" {
		t.Fatal("enabled helper mutated the input preset")
	}
	if updated.GetNullString() != "NULL" {
		t.Fatalf("enabled NullString = %q, want existing NULL spelling", updated.GetNullString())
	}
	sqlNull, err := updated.FormatToplevelColumn(gcvctor.NullFromCode(sppb.TypeCode_STRING))
	if err != nil {
		t.Fatal(err)
	}
	lit, err := updated.FormatToplevelColumn(gcvctor.StringValue(showNullsLiteral))
	if err != nil {
		t.Fatal(err)
	}
	angle, err := updated.FormatToplevelColumn(gcvctor.StringValue("<NULL>"))
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := updated.FormatToplevelColumn(gcvctor.StringValue(`"NULL"`))
	if err != nil {
		t.Fatal(err)
	}
	already := strconv.Quote(`"NULL"`)
	escaped, err := updated.FormatToplevelColumn(gcvctor.StringValue(already))
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := updated.FormatToplevelColumn(gcvctor.StringValue("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if sqlNull != "NULL" || lit != strconv.Quote(showNullsLiteral) || angle != "<NULL>" || quoted != strconv.Quote(`"NULL"`) || escaped != strconv.Quote(already) || ordinary != "abc" {
		t.Fatalf("enabled seam sql=%q lit=%q angle=%q quoted=%q escaped=%q ordinary=%q", sqlNull, lit, angle, quoted, escaped, ordinary)
	}
}

func TestApplyShowNullsDisplayTargetedQuoting(t *testing.T) {
	t.Parallel()
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := applyShowNullsDisplay(fc, true, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}

	alreadyEscaped := strconv.Quote(`"NULL"`)
	cases := []struct {
		name string
		val  spanner.GenericColumnValue
		want string
	}{
		{"sql null", gcvctor.NullFromCode(sppb.TypeCode_STRING), "NULL"},
		{"literal NULL", gcvctor.StringValue(showNullsLiteral), strconv.Quote(showNullsLiteral)},
		{"quote-prefixed NULL", gcvctor.StringValue(`"NULL"`), strconv.Quote(`"NULL"`)},
		{"already escaped output", gcvctor.StringValue(alreadyEscaped), strconv.Quote(alreadyEscaped)},
		{"angle", gcvctor.StringValue("<NULL>"), "<NULL>"},
		{"ordinary", gcvctor.StringValue("abc"), "abc"},
		{"empty", gcvctor.StringValue(""), ""},
		{"nullish", gcvctor.StringValue("NULLISH"), "NULLISH"},
		{"lowercase null", gcvctor.StringValue("null"), "null"},
		{"leading space NULL", gcvctor.StringValue(" NULL"), " NULL"},
		{"quote plus backslash", gcvctor.StringValue(`"\`), strconv.Quote(`"\`)},
		{"quote plus tab", gcvctor.StringValue("\"\t"), strconv.Quote("\"\t")},
		{"quote plus newline", gcvctor.StringValue("\"\n"), strconv.Quote("\"\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := updated.FormatToplevelColumn(tc.val)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCLIShowNullsDefaultIdentityAndEnabledContract(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustShowNullsTyped(t)

	off := showNullsSysVars(enums.DisplayModeTable, false, enums.StyledModeFalse)
	on := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeFalse)
	defaultOff := newSystemVariablesWithDefaults()
	defaultOff.Display.StyledOutput = enums.StyledModeFalse

	gotOff := printShowNulls(t, &off, header, md, rawRows)
	gotDefault := printShowNulls(t, &defaultOff, header, md, rawRows)
	if diff := cmp.Diff(gotOff, gotDefault); diff != "" {
		t.Fatalf("explicit FALSE must match default unset (-explicit +default):\n%s", diff)
	}
	if strings.Contains(gotOff, "\033[") {
		t.Fatalf("colors-off TABLE leaked ANSI:\n%s", gotOff)
	}
	if !strings.Contains(gotOff, "| NULL     | NULL       | <NULL>    |") {
		t.Fatalf("default TABLE should collide SQL NULL with STRING NULL:\n%s", gotOff)
	}

	gotOn := printShowNulls(t, &on, header, md, rawRows)
	if !strings.Contains(gotOn, `| NULL     | "NULL"     | <NULL>    |`) {
		t.Fatalf("enabled TABLE contract mismatch:\n%s", gotOn)
	}
	if !strings.Contains(gotOn, `[NULL, "NULL"]`) {
		t.Fatalf("nested ARRAY should distinguish SQL NULL from STRING NULL:\n%s", gotOn)
	}

	cellsOff, err := deriveDisplayRows(&off, &TypedRows{Metadata: md, Rows: rawRows})
	if err != nil {
		t.Fatal(err)
	}
	cellsOn, err := deriveDisplayRows(&on, &TypedRows{Metadata: md, Rows: rawRows})
	if err != nil {
		t.Fatal(err)
	}
	alreadyEscaped := strconv.Quote(`"NULL"`)
	wantOff := []string{"NULL", "NULL", "<NULL>", `"NULL"`, "", "NULL", "NULL", "", "NULL", "[]", `[NULL, NULL, "NULL"]`}
	wantOn := []string{"NULL", strconv.Quote(showNullsLiteral), "<NULL>", strconv.Quote(`"NULL"`), "", "NULL", "NULL", "", "NULL", "[]", `[NULL, "NULL", ` + strconv.Quote(`"NULL"`) + `]`}
	for i, want := range wantOff {
		if got := cellsOff[0][i].RawText(); got != want {
			t.Errorf("default cell %d = %q, want %q", i, got, want)
		}
	}
	for i, want := range wantOn {
		if got := cellsOn[0][i].RawText(); got != want {
			t.Errorf("enabled cell %d = %q, want %q", i, got, want)
		}
	}
	if cellsOff[0][14].RawText() != "abc" || cellsOn[0][14].RawText() != "abc" {
		t.Errorf("ordinary STRING abc changed: off=%q on=%q", cellsOff[0][14].RawText(), cellsOn[0][14].RawText())
	}
	if cellsOff[0][15].RawText() != alreadyEscaped {
		t.Errorf("default already-escaped = %q, want %q", cellsOff[0][15].RawText(), alreadyEscaped)
	}
	if cellsOn[0][15].RawText() != strconv.Quote(alreadyEscaped) {
		t.Errorf("enabled already-escaped = %q, want %q", cellsOn[0][15].RawText(), strconv.Quote(alreadyEscaped))
	}
	if cellsOn[0][11].RawText() != `[NULL, "NULL"]` {
		t.Errorf("enabled STRUCT = %q, want [NULL, \"NULL\"]", cellsOn[0][11].RawText())
	}
	if cellsOn[0][12].RawText() != "null" {
		t.Errorf("JSON JSON-null = %q, want null", cellsOn[0][12].RawText())
	}
	if cellsOn[0][13].RawText() != "NULL" {
		t.Errorf("SQL-null JSON = %q, want NULL", cellsOn[0][13].RawText())
	}
	if _, ok := cellsOn[0][0].(format.NoWrapCell); !ok {
		t.Fatalf("enabled SQL NULL cell type %T, want NoWrapCell", cellsOn[0][0])
	}
	if _, ok := cellsOn[0][1].(format.NoWrapCell); ok {
		t.Fatal("STRING NULL must not become NoWrapCell")
	}

	styledOn := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeTrue)
	styledOut := printShowNulls(t, &styledOn, header, md, rawRows)
	if !strings.Contains(styledOut, "\033[2mNULL\033[0m") {
		t.Fatalf("styled SQL NULL should keep dim around NULL:\n%s", styledOut)
	}
	if strings.Contains(styledOut, "\033[2m\"NULL\"") {
		t.Fatalf("quoted STRING NULL must not use NULL dim styling:\n%s", styledOut)
	}

	vertOn := showNullsSysVars(enums.DisplayModeVertical, true, enums.StyledModeFalse)
	vertOff := showNullsSysVars(enums.DisplayModeVertical, false, enums.StyledModeFalse)
	gotVertOn := printShowNulls(t, &vertOn, header, md, rawRows)
	gotVertOff := printShowNulls(t, &vertOff, header, md, rawRows)
	if !strings.Contains(gotVertOff, "null_col: NULL") || !strings.Contains(gotVertOff, "string_col: NULL") {
		t.Fatalf("VERTICAL default collision missing:\n%s", gotVertOff)
	}
	if !strings.Contains(gotVertOn, "null_col: NULL") || !strings.Contains(gotVertOn, `string_col: "NULL"`) || !strings.Contains(gotVertOn, "angle_col: <NULL>") {
		t.Fatalf("VERTICAL enabled contract mismatch:\n%s", gotVertOn)
	}
	if !strings.Contains(gotVertOn, `quoted_null: `+strconv.Quote(`"NULL"`)) || !strings.Contains(gotVertOn, "ordinary: abc") {
		t.Fatalf("VERTICAL enabled quote-prefix/ordinary mismatch:\n%s", gotVertOn)
	}

	for _, mode := range []enums.DisplayMode{
		enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment,
	} {
		sv := showNullsSysVars(mode, true, enums.StyledModeFalse)
		got := printShowNulls(t, &sv, header, md, rawRows)
		if !strings.Contains(got, `"NULL"`) || !strings.Contains(got, "<NULL>") {
			t.Fatalf("%s enabled output missing distinction:\n%s", mode, got)
		}
	}
}

func TestCLIShowNullsExcludedFormatsUnchanged(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustShowNullsTyped(t)

	for _, mode := range []enums.DisplayMode{
		enums.DisplayModeCSV, enums.DisplayModeTSV, enums.DisplayModeJSONL,
		enums.DisplayModeSQLInsert, enums.DisplayModeTab, enums.DisplayModeHTML, enums.DisplayModeXML,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			off := showNullsSysVars(mode, false, enums.StyledModeFalse)
			on := showNullsSysVars(mode, true, enums.StyledModeFalse)
			gotOff := printShowNulls(t, &off, header, md, rawRows)
			gotOn := printShowNulls(t, &on, header, md, rawRows)
			if diff := cmp.Diff(gotOff, gotOn); diff != "" {
				t.Fatalf("CLI_SHOW_NULLS must not change %s (-off +on):\n%s", mode, diff)
			}
			if gotOff == "" {
				t.Fatal("expected non-empty excluded-format output")
			}
		})
	}
}

func TestCLIShowNullsSQLExportFallbackKeepsExistingBytes(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustShowNullsTyped(t)

	tableOff := showNullsSysVars(enums.DisplayModeTable, false, enums.StyledModeFalse)
	want := printShowNulls(t, &tableOff, header, md, rawRows)

	sqlOn := showNullsSysVars(enums.DisplayModeSQLInsert, true, enums.StyledModeFalse)
	var buf bytes.Buffer
	if err := printTableData(&sqlOn, 0, &buf, &Result{
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
		t.Fatalf("SQL-export fallback with SHOW_NULLS must keep default TABLE bytes (-want +got):\n%s", diff)
	}
}

func TestCLIShowNullsStreamingMatchesBuffered(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustShowNullsTyped(t)
	sv := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeFalse)

	buffered := printShowNulls(t, &sv, header, md, rawRows)

	render, err := prepareFormatConfig("SELECT * FROM Items", &sv, queryRenderingFrom(&sv))
	if err != nil {
		t.Fatal(err)
	}
	if render.Spanvalue.GetNullString() != "NULL" {
		t.Fatalf("streaming FormatConfig NullString = %q", render.Spanvalue.GetNullString())
	}
	transform := spannerRowToRow(render.Spanvalue, render.TypeStyles, render.NullStyle)
	eager, err := transform(rawRows[0])
	if err != nil {
		t.Fatal(err)
	}
	var streamed bytes.Buffer
	if err := printTableData(&sv, 0, &streamed, &Result{
		TableHeader: header,
		Body:        PresentationBody([]Row{eager}),
	}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(buffered, streamed.String()); diff != "" {
		t.Fatalf("streaming-equivalent presentation vs buffered typed (-buffered +streamed):\n%s", diff)
	}
}

func TestCLIShowNullsThenReturnUsesTypedPolicy(t *testing.T) {
	t.Parallel()
	md, rawRows, header := mustShowNullsTyped(t)
	sv := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeFalse)
	got, err := renderDMLReturnedRows(&sv, header, md, rawRows)
	if err != nil {
		t.Fatal(err)
	}
	want := printShowNulls(t, &sv, header, md, rawRows)
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Fatalf("THEN RETURN must match typed TABLE policy (-want +got):\n%s", diff)
	}
}

func TestCLIShowNullsPresentationRowsUnchanged(t *testing.T) {
	t.Parallel()
	sv := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeFalse)
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
	if !strings.Contains(buf.String(), "NULL") {
		t.Fatalf("expected presentation NULL text:\n%s", buf.String())
	}
}

func TestCLIShowNullsPreservesProtoEnumPlugins(t *testing.T) {
	t.Parallel()
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(protos.File_singer_proto)},
	}
	base, err := decoder.FormatConfigWithProto(fds, false)
	if err != nil {
		t.Fatal(err)
	}
	seamed, err := applyShowNullsDisplay(base, true, enums.DisplayModeTable)
	if err != nil {
		t.Fatal(err)
	}

	enumVal := gcvctor.EnumValue("examples.spanner.music.Genre", int64(protos.Genre_POP))
	protoVal := gcvctor.ProtoValue("examples.spanner.music.SingerInfo", mustMarshalSinger(t))
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
		t.Fatalf("enum formatting changed (-base +seamed):\n%s", diff)
	}
	if seamEnum == "NULL" || seamEnum == `"NULL"` {
		t.Fatalf("enum formatted as null-like %q", seamEnum)
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
		t.Fatalf("proto formatting changed (-base +seamed):\n%s", diff)
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

func TestCLIShowNullsSetShowResetLocal(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	got, err := sv.Get("CLI_SHOW_NULLS")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("CLI_SHOW_NULLS", "FALSE"), got); diff != "" {
		t.Fatalf("default SHOW mismatch (-want +got):\n%s", diff)
	}
	if err := sv.SetFromSimple("CLI_SHOW_NULLS", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if !sv.Display.ShowNulls {
		t.Fatal("SET did not enable ShowNulls")
	}
	got, err = sv.Get("CLI_SHOW_NULLS")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("CLI_SHOW_NULLS", "TRUE"), got); diff != "" {
		t.Fatalf("SHOW after SET mismatch (-want +got):\n%s", diff)
	}
	if err := sv.Reset("CLI_SHOW_NULLS"); err != nil {
		t.Fatal(err)
	}
	if sv.Display.ShowNulls {
		t.Fatal("RESET did not restore FALSE")
	}

	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_SHOW_NULLS", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_SHOW_NULLS"); got != "TRUE" {
		t.Fatalf("SET LOCAL = %s", got)
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_SHOW_NULLS"); got != "FALSE" {
		t.Fatalf("LOCAL should restore after COMMIT: %s", got)
	}
}

func TestPrepareAndClientSideFormatContextHonorShowNulls(t *testing.T) {
	t.Parallel()
	sv := showNullsSysVars(enums.DisplayModeTable, true, enums.StyledModeFalse)
	render, err := prepareFormatConfig("SELECT 1", &sv, queryRenderingFrom(&sv))
	if err != nil {
		t.Fatal(err)
	}
	if render.Spanvalue.GetNullString() != "NULL" {
		t.Fatalf("prepareFormatConfig TABLE NullString = %q", render.Spanvalue.GetNullString())
	}
	lit, err := render.Spanvalue.FormatToplevelColumn(gcvctor.StringValue(showNullsLiteral))
	if err != nil {
		t.Fatal(err)
	}
	if lit != `"NULL"` {
		t.Fatalf("prepareFormatConfig TABLE STRING NULL = %q", lit)
	}

	csv := showNullsSysVars(enums.DisplayModeCSV, true, enums.StyledModeFalse)
	csvRender, err := prepareFormatConfig("SELECT 1", &csv, queryRenderingFrom(&csv))
	if err != nil {
		t.Fatal(err)
	}
	if csvRender.Spanvalue.GetNullString() != "NULL" {
		t.Fatalf("prepareFormatConfig CSV NullString = %q", csvRender.Spanvalue.GetNullString())
	}
	csvLit, err := csvRender.Spanvalue.FormatToplevelColumn(gcvctor.StringValue(showNullsLiteral))
	if err != nil {
		t.Fatal(err)
	}
	if csvLit != "NULL" {
		t.Fatalf("prepareFormatConfig CSV STRING NULL = %q, want unquoted", csvLit)
	}

	fc, _, err := clientSideFormatContext(&sv)
	if err != nil {
		t.Fatal(err)
	}
	if fc.GetNullString() != "NULL" {
		t.Fatalf("clientSideFormatContext TABLE NullString = %q", fc.GetNullString())
	}
	clientLit, err := fc.FormatToplevelColumn(gcvctor.StringValue(showNullsLiteral))
	if err != nil {
		t.Fatal(err)
	}
	if clientLit != `"NULL"` {
		t.Fatalf("clientSideFormatContext TABLE STRING NULL = %q", clientLit)
	}

	sqlSV := showNullsSysVars(enums.DisplayModeSQLInsert, true, enums.StyledModeFalse)
	sqlFC, vfm, err := clientSideFormatContext(&sqlSV)
	if err != nil {
		t.Fatal(err)
	}
	if vfm != format.DisplayValues {
		t.Fatalf("SQL client-side value mode = %v", vfm)
	}
	if sqlFC.GetNullString() != "NULL" {
		t.Fatalf("client-side SQL fallback must keep NullString NULL, got %q", sqlFC.GetNullString())
	}
}

func mustMarshalSinger(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCLIShowNullsHelperRejectsEmptyMarkerConfig(t *testing.T) {
	t.Parallel()
	// Validate must surface a real error if a future edit clears NullString.
	fc := spanvalue.SpannerCLICompatibleFormatConfig().Clone()
	fc.NullString = ""
	if err := fc.Validate(); err == nil {
		t.Fatal("expected Validate error for empty NullString")
	}
}
