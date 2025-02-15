package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/protobuf/encoding/prototext"

	"google.golang.org/genai"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"

	"github.com/apstndb/genaischema"
)

//go:embed official_docs/*
var docs embed.FS

type output struct {
	CandidateStatements []*statement `json:"candidateStatements" description:"Candidate statements" minItems:"1" maxItems:"5" required:"true"`
	Statement           *statement   `json:"statement" description:"Final result, select from candidateStatements" required:"true"`
}

type statement struct {
	Text                string `json:"text" description:"Query text. It should be formatted and indented. It must be terminated by semicolon" required:"true"`
	FixedText           string `json:"fixedText" description:"text or fixed text if needed" required:"true"`
	Reason              string `json:"reason" description:"Reason of selection" required:"true"`
	SyntaxDescription   string `json:"syntaxDescription" description:"A long description of text in syntax, detailed and strictly" required:"true"`
	SemanticDescription string `json:"semanticDescription" description:"Description of text in semantics. Must describe how the request is achieved" required:"true"`
}

var outputSchema = lo.Must(genaischema.GenerateForType[output]())

func geminiComposeQuery(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, model, s string) (*output, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: "us-central1",
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(resp.GetProtoDescriptors(), &fds)
	if err != nil {
		return nil, err
	}

	var parts []*genai.Part
	err = fs.WalkDir(docs, "official_docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "README.md") {
			return nil
		}

		b, err := docs.ReadFile(path)
		if err != nil {
			return err
		}

		parts = append(parts, &genai.Part{InlineData: &genai.Blob{
			Data:     b,
			MIMEType: "text/markdown",
		}})
		return nil
	})
	if err != nil {
		return nil, err
	}

	contents := []*genai.Content{{Parts: sliceOf(genai.NewPartFromText(s))}}
	if len(parts) > 0 {
		contents = append(contents, &genai.Content{Parts: parts})
	}

	response, err := client.Models.GenerateContent(ctx, model,
		contents,
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   outputSchema,
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: `
Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
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
` + fmt.Sprintf("```\n%v\n```", prototext.Format(&fds)),
				},
				},
			},
		})
	if err != nil {
		return nil, err
	}

	// pp.Print(response)

	var result output
	err = json.Unmarshal([]byte(response.Candidates[0].Content.Parts[0].Text), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
