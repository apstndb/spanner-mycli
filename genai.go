package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
		genai.Text(s),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: `
Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
GoogleSQL syntax is not PostgreSQL syntax.
The output must be valid.
The output should be terminated with terminating semicolon.
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
