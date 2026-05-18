package mycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/k0kubun/pp/v3"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
	"google.golang.org/protobuf/encoding/protojson"
)

type queryProfiles struct {
	RawQueryPlan jsontext.Value  `json:"queryPlan"`
	QueryPlan    *sppb.QueryPlan `json:"-"`
	QueryStats   QueryStats      `json:"queryStats"`
	Fprint       string          `json:"fprint"`
}

type queryProfilesRow struct {
	IntervalEnd     time.Time        `spanner:"INTERVAL_END"`
	TextFingerprint int64            `spanner:"TEXT_FINGERPRINT"`
	LatencySeconds  float64          `spanner:"LATENCY_SECONDS"`
	RawQueryProfile spanner.NullJSON `spanner:"QUERY_PROFILE"`
	QueryProfile    *queryProfiles   `spanner:"-"`
}

var (
	t    = template.New("temp")
	temp = lo.Must(t.Parse(
		`
interval_end:                 {{.IntervalEnd}}
text_fingerprint:             {{.TextFingerprint}}
{{with .QueryProfile.QueryStats -}}
elapsed_time:                 {{.ElapsedTime}}
cpu_time:                     {{.CPUTime}}
rows_returned:                {{.RowsReturned}}
deleted_rows_scanned:         {{.DeletedRowsScanned}}
optimizer_version:            {{.OptimizerVersion}}
optimizer_statistics_package: {{.OptimizerStatisticsPackage}}
{{end}}`))
)

type ShowQueryProfilesStatement struct{}

func (s *ShowQueryProfilesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, `SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`)
	}

	stmt := spanner.Statement{
		SQL: `SELECT INTERVAL_END, TEXT_FINGERPRINT, LATENCY_SECONDS, PARSE_JSON(QUERY_PROFILE) AS QUERY_PROFILE FROM SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`,
	}

	iter, _ := session.RunQuery(ctx, stmt)

	rows, _, _, _, _, err := consumeRowIterCollect(iter, toQpr)
	if err != nil {
		return nil, err
	}

	var resultRows []Row
	for _, row := range rows {
		rows, _, appendices, err := processPlanWithoutStats(row.QueryProfile.QueryPlan, session.systemVariables.Display.ExplainFormat, session.systemVariables.Display.ExplainWrapWidth, resolveExplainPrintSections(session.systemVariables, nil))
		if err != nil {
			return nil, err
		}

		maxIDLength := 2
		for _, row := range rows {
			maxIDLength = max(maxIDLength, len(row[0].RawText()) /* ID */)
		}

		pprinter := pp.New()
		pprinter.SetColoringEnabled(false)

		tree := strings.Join(slices.Collect(loi.Map(
			slices.Values(rows),
			func(r Row) string {
				return tabwrap.FillLeft(r[0].RawText() /* ID */, maxIDLength) + " | " + r[1].RawText() /* Plan */
			},
		)), "\n")

		resultRows = append(resultRows, toRow(row.QueryProfile.QueryStats.QueryText+"\n"+tabwrap.FillRight("ID", maxIDLength)+" | Plan\n"+tree+
			formatQueryProfileAppendices(appendices)+"\n"+
			formatStats(row)))
	}

	return &Result{
		TableHeader:  toTableHeader("Plan"),
		Rows:         resultRows,
		AffectedRows: len(resultRows),
	}, nil
}

func formatQueryProfileAppendices(appendices []ResultAppendix) string {
	var b strings.Builder
	for _, appendix := range appendices {
		if len(appendix.Lines) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		title := appendix.Title
		if title == "Predicates(identified by ID):" {
			title = "Predicates:"
		}
		b.WriteString(title)
		b.WriteString("\n")
		b.WriteString(strings.Join(appendix.Lines, "\n"))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n" + b.String()
}

type ShowQueryProfileStatement struct {
	Fprint int64
}

func (s *ShowQueryProfileStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, `SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`)
	}

	stmt := spanner.Statement{
		SQL: `SELECT INTERVAL_END, TEXT_FINGERPRINT, LATENCY_SECONDS, PARSE_JSON(QUERY_PROFILE) AS QUERY_PROFILE
FROM SPANNER_SYS.QUERY_PROFILES_TOP_HOUR
WHERE TEXT_FINGERPRINT = @fprint
ORDER BY INTERVAL_END DESC`,
		Params: map[string]interface{}{"fprint": s.Fprint},
	}

	iter, _ := session.RunQuery(ctx, stmt)

	qprs, _, _, _, _, err := consumeRowIterCollect(iter, toQpr)
	if err != nil {
		return nil, err
	}

	qpr, ok := lo.First(qprs)
	if !ok {
		return nil, errors.New("empty result")
	}

	return buildExplainAnalyzeResult(session.systemVariables, qpr.QueryProfile.QueryPlan, qpr.QueryProfile.QueryStats,
		enums.ExplainFormatUnspecified, 0, nil)
}

func formatStats(stats *queryProfilesRow) string {
	var sb strings.Builder
	if stats == nil {
		return ""
	}

	err := temp.Execute(&sb, stats)
	if err != nil {
		slog.Error("error occurred", "err", err)
		return ""
	}

	return sb.String()
}

func toQpr(row *spanner.Row) (*queryProfilesRow, error) {
	var qpr queryProfilesRow
	if err := row.ToStruct(&qpr); err != nil {
		return nil, err
	}

	var profile queryProfiles
	err := json.Unmarshal([]byte(qpr.RawQueryProfile.String()), &profile)
	if err != nil {
		return nil, err
	}
	qpr.QueryProfile = &profile

	var queryPlan sppb.QueryPlan
	err = protojson.Unmarshal(profile.RawQueryPlan, &queryPlan)
	if err != nil {
		return nil, err
	}
	profile.QueryPlan = &queryPlan

	return &qpr, nil
}
