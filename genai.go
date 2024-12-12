package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/protobuf/encoding/prototext"

	"google.golang.org/genai"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

func geminiComposeQuery(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, s string) (string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: "us-central1",
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return "", err
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(resp.GetProtoDescriptors(), &fds)
	if err != nil {
		return "", err
	}

	response, err := client.Models.GenerateContent(ctx, "gemini-2.0-flash-exp",
		genai.PartSlice{
			genai.Text(s),
		},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.Text(
				`
Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
GoogleSQL syntax is not PostgreSQL syntax.
The output must be valid.
The output should be terminated with terminating semicolon.
NULL_FILTERED indexes can be dropped using DROP INDEX statement, not DROP NULL_FILTERED INDEX statement.
Here is the DDL.
` +
					fmt.Sprintf("```\n%v\n```", strings.Join(resp.GetStatements(), ";\n")+";") + `
Here is the Proto Descriptors.
` + fmt.Sprintf("```\n%v\n```", prototext.Format(&fds))).ToContent(),
		})
	if err != nil {
		return "", err
	}

	// pp.Print(response)

	re := regexp.MustCompile("(?s)```[^\\n]*\\n(.*)\\n```")
	return re.FindStringSubmatch(response.Candidates[0].Content.Parts[0].Text)[1], nil
}

func geminiExplain(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project string, sql string, queryPlan *sppb.QueryPlan) (string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: "us-central1",
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return "", err
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(resp.GetProtoDescriptors(), &fds)
	if err != nil {
		return "", err
	}

	response, err := client.Models.GenerateContent(ctx, "gemini-2.0-flash-exp",
		genai.PartSlice{
			genai.Text(`Explain possible bottleneck of the query and show solutions both of query and schema.
The output should be shorter than 30 lines after wrapping.
Remember all indexes already contains primary keys of their table so it is no need to add storing columns.`),
		},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.Text(
				`
Answer must be a valid Spanner GoogleSQL syntax.
You must understand the schema DDL of the database before you compose SQL.
DisplayName in the answer text should indicate Index value in "$DisplayName (ID: $Index)" format ($DisplayName and $ID is a field value of PlanNode).
You must see already created index in DDL to avoid to create the index with the same definition.

You should never create a covering index with keys in the same order as the primary key. this is pointless and creates overhead.
Without back-join, table scan with seek scan by primary keys are optimal and not bottleneck.

"SELECT *" is not bottleneck unless it causes back-join.
Back-join is JOINs to lookup base tables. They are always appeared in the query plan so there is no join of index and its base table in query plan, there is no back-join.
You should check index and base table relationship in the schema DDL.
If there is no back-join, the index is already covering index so not needed to change.
Primary keys of the table are always stored in all indexes, so they are no need to be covered in covering indexes.
Remember all primary key columns of the table and parents are already included in indexes.
Remember index scan scans only index. Lookup is not occurred if there is no explicit table scan.

Stream Aggregate with Limit can do early termination of underlying scans, so it is not bottleneck even if full scans.
Usually, Hash Aggregate is result of lacking suitable indexes and it causes full scans.
Hash Aggregate needs to build hash table, and it prevents early termination using limit.

If you suggests covering index, Show both of CREATE INDEX with STORING and ALTER INDEX ADD STORED COLUMN with already created indexes.

Don't include primary keys of the table of the index in storing columns. IT WILL BE INVALID DDL!.

There is no ALTER TABLE ADD INDEX statement in Spanner GoogleSQL DDL so you should suggest CREATE INDEX statement if needed.

Covering indexes must be named as "${TableName}By${KeyColumns}Covering" where ${TableName} is the table name and ${KeyColumns} are column names of index key column.
Column names of storing columns must not be encoded in index names.
For example, "CREATE INDEX SongsBySongGenreCovering ON Songs(SongGenre) STORING (Duration)" is correct and "CREATE INDEX SongsBySongGenreDurationCovering ON Songs(SongGenre) STORING (Duration)" is not correct.

If you expect to use the specific index, you must explicitly use the FORCE_INDEX hint. You must not rely on automatic index selection of the optimizer.
FORCE_INDEX hint is "$TABLE_NAME@{FORCE_INDEX=$INDEX_NAME}"($TABLE_NAME is the table name, $INDEX_NAME is the target index name).
You should prefer STORING than adding suffix of primary keys, if the column is not used for scans. (e.g. only used for SELECT or aggregate function)

Here is the Spanner query.
` + fmt.Sprintf("```\n%v\n```", sql) + `
Here is the query plan in protojson of spanner.v1.ResultSetStats.QueryPlan.
` + fmt.Sprintf("```\n%v\n```", protojson.Format(queryPlan)) + `
Here is the schema DDL of the database.
` + fmt.Sprintf("```\n%v\n```", strings.Join(resp.GetStatements(), ";\n")+";") + `
Here is the Proto Descriptors.
` + fmt.Sprintf("```\n%v\n```", prototext.Format(&fds))).ToContent(),
		})
	if err != nil {
		return "", err
	}

	// pp.Print(response)

	return response.Candidates[0].Content.Parts[0].Text, nil
}
