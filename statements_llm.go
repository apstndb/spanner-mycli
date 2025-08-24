package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/protobuf/encoding/prototext"

	"google.golang.org/genai"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"

	"github.com/apstndb/genaischema"
)

type output struct {
	CandidateStatements []*statement `json:"candidateStatements" description:"Candidate statements" minItems:"1" maxItems:"5" required:"true"`
	Statement           *statement   `json:"statement" description:"Final result, select from candidateStatements" required:"true"`
	ErrorDescription    string       `json:"errorDescription" description:"Description of error. Available only if input contains error message"`
}

type statement struct {
	Text                string `json:"text" description:"Query text. It should be formatted and indented. It must be terminated by semicolon" required:"true"`
	FixedText           string `json:"fixedText" description:"text or fixed text if needed" required:"true"`
	Reason              string `json:"reason" description:"Reason of selection" required:"true"`
	SyntaxDescription   string `json:"syntaxDescription" description:"A long description of text in syntax, detailed and strictly" required:"true"`
	SemanticDescription string `json:"semanticDescription" description:"Description of text in semantics. Must describe how the request is achieved" required:"true"`
}

//go:embed official_docs/*
var docs embed.FS

type GeminiStatement struct {
	Text string
}

func (s *GeminiStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	composed, err := geminiComposeQuery(ctx, resp, session.systemVariables.VertexAIProject, session.systemVariables.VertexAIModel, s.Text)
	if err != nil {
		return nil, err
	}

	return &Result{
		PreInput: composed.Statement.Text,
		Rows: slices.Concat(
			lo.Ternary(composed.ErrorDescription != "",
				sliceOf(toRow("errorDescription", composed.ErrorDescription)),
				nil),
			sliceOf(
				toRow("text", composed.Statement.Text),
				toRow("semanticDescription", composed.Statement.SemanticDescription),
				toRow("syntaxDescription", composed.Statement.SyntaxDescription))),
		TableHeader: toTableHeader("Column", "Value"),
	}, nil
}

func readFiles(fsys fs.FS, root string, pred func(string, fs.DirEntry) bool) ([][]byte, error) {
	var files [][]byte
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !pred(path, d) {
			return nil
		}
		b, err := docs.ReadFile(path)
		if err != nil {
			return err
		}

		files = append(files, b)
		return nil
	})
	return files, err
}

func geminiComposeQuery(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, model, s string) (*output, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:     project,
		Location:    "us-central1",
		Backend:     genai.BackendVertexAI,
		HTTPOptions: genai.HTTPOptions{APIVersion: "v1"},
	})
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(resp.GetProtoDescriptors(), &fds)
	if err != nil {
		return nil, err
	}

	fileContents, err := readFiles(docs, "official_docs", func(path string, d fs.DirEntry) bool {
		return !d.IsDir() && strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, "README.md")
	})
	if err != nil {
		return nil, err
	}

	parts := sliceOf(genai.NewPartFromText(s))
	for _, content := range fileContents {
		parts = append(parts, genai.NewPartFromBytes(content, "text/markdown"))
	}

	systemPrompt := `
Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
Prefer SQL query rather than GQL query unless GQL is explicitly requested.
GoogleSQL syntax is not PostgreSQL syntax.
Remember GQL requires output column names.
Remember GQL is neither of GraphQL nor other graph query languages like Cypher.
If you are outputting GQL, you should double-check your DDL definitions to ensure that the edge directions are correct.
The output must be valid query.
The output must be terminated with terminating semicolon.
NULL_FILTERED indexes can be dropped using DROP INDEX statement, not DROP NULL_FILTERED INDEX statement.
Here is the DDL.
` +
		fmt.Sprintf("```\n%v\n```", strings.Join(resp.GetStatements(), ";\n")+";") + `
Here is the prototext of File Proto Descriptors.
` + fmt.Sprintf("```\n%v\n```", prototext.Format(&fds))

	return genaischema.GenerateObjectContent[*output](ctx, client, model,
		[]*genai.Content{{Role: genai.RoleUser, Parts: parts}},
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: sliceOf(genai.NewPartFromText(systemPrompt)),
			},
		})
}
