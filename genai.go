package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/protobuf/encoding/prototext"

	"google.golang.org/genai"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

//go:embed official_docs/*
var docs embed.FS

func geminiComposeQuery(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, model, s string) (string, error) {
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

	var parts []*genai.Part
	err = fs.WalkDir(docs, "official_docs", func(path string, d fs.DirEntry, err error) error {
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
		return "", err
	}

	var contents []*genai.Content
	contents = append(contents, genai.Text(s)[0])
	if len(parts) > 0 {
		contents = append(contents, &genai.Content{Parts: parts})
	}

	response, err := client.Models.GenerateContent(ctx, model,
		// response, err := client.Models.GenerateContent(ctx, "gemini-2.0-pro-exp-02-05",
		contents,
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: `
Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
GoogleSQL syntax is not PostgreSQL syntax.
Remember GQL requires output column names.
Remember GQL is not GraphQL.
The output must be valid query.
The output must be terminated with terminating semicolon.
NULL_FILTERED indexes can be dropped using DROP INDEX statement, not DROP NULL_FILTERED INDEX statement.
Here is the DDL.
` +
					fmt.Sprintf("```\n%v\n```", strings.Join(resp.GetStatements(), ";\n")+";") + `
Here is the Proto Descriptors.
` + fmt.Sprintf("```\n%v\n```", prototext.Format(&fds)),
				},
				},
			},
		})
	if err != nil {
		return "", err
	}

	// pp.Print(response)

	re := regexp.MustCompile("(?s)```[^\\n]*\\n(.*)\\n```")
	return re.FindStringSubmatch(response.Candidates[0].Content.Parts[0].Text)[1], nil
}
