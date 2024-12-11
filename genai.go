package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/firebase/genkit/go/ai"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/firebase/genkit/go/plugins/vertexai"
)

var vertexaiInited bool

func geminiComposeQuery(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, s string) (string, error) {
	if !vertexaiInited {
		if err := vertexai.Init(ctx, &vertexai.Config{
			ProjectID: project,
		}); err != nil {
			return "", err
		}
	}
	vertexaiInited = true

	model := vertexai.Model("gemini-1.5-flash")
	responseText, err := ai.GenerateText(ctx, model,
		ai.WithTextPrompt(s),
		ai.WithSystemPrompt(fmt.Sprintf("Answer in the plain-text executable valid Spanner GoogleSQL syntax or Spanner Graph GQL syntax with terminating semicolon. \nHere is the DDL.\n```\n%v\n```", strings.Join(resp.GetStatements(), ";\n")+";")),
		ai.WithOutputFormat(ai.OutputFormatText),
	)
	if err != nil {
		return "", err
	}

	log.Print(responseText)

	re := regexp.MustCompile("(?s)```[^\\n]*\\n(.*)\\n```")
	return re.FindStringSubmatch(responseText)[1], nil
}
