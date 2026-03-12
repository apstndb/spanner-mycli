package mycli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/genai"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

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

// devKnowledgeAPIKey returns the API key for Developer Knowledge API,
// preferring DEVELOPERKNOWLEDGE_API_KEY over GOOGLE_API_KEY.
func devKnowledgeAPIKey() string {
	if v := os.Getenv("DEVELOPERKNOWLEDGE_API_KEY"); v != "" {
		return v
	}
	return os.Getenv("GOOGLE_API_KEY")
}

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

	project := session.systemVariables.Feature.VertexAIProject
	location := session.systemVariables.Feature.VertexAILocation
	model := session.systemVariables.Feature.VertexAIModel

	// Build the doc cache with embedded docs
	cacheOpts := []docCacheOption{}
	apiKey := devKnowledgeAPIKey()
	var apiClient *devKnowledgeClient
	if apiKey != "" {
		apiClient = newDevKnowledgeClient(apiKey)
		searcher := &devKnowledgeDocSearcher{client: apiClient}
		cacheOpts = append(cacheOpts,
			withDocFetcher(func(ctx context.Context, name string) (string, error) {
				return searcher.GetDocument(ctx, name)
			}),
			withDocBatchFetcher(func(ctx context.Context, names []string) ([]DocResult, error) {
				return searcher.BatchGetDocuments(ctx, names)
			}),
			withDocAPISearcher(func(ctx context.Context, query string) ([]DocSearchResult, error) {
				return searcher.Search(ctx, query)
			}),
		)
		slog.Debug("Developer Knowledge API enabled for documentation")
	} else {
		slog.Debug("No API key set, using embedded docs only")
	}

	cache, err := newDocCache(cacheOpts...)
	if err != nil {
		return nil, fmt.Errorf("create doc cache: %w", err)
	}
	defer cache.Close()

	if err := loadEmbeddedDocs(cache); err != nil {
		return nil, fmt.Errorf("load embedded docs: %w", err)
	}

	composed, err := geminiComposeQueryWithTools(ctx, resp, project, location, model, s.Text, cache, apiKey != "")
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

// geminiSystemPrompt builds the system prompt with DDL and proto descriptors.
func geminiSystemPrompt(resp *adminpb.GetDatabaseDdlResponse, fds *descriptorpb.FileDescriptorSet) string {
	return `You are a Cloud Spanner query composer. Your task is to compose valid queries based on the user's request.

Rules:
- Answer in valid Spanner GoogleSQL syntax or valid Spanner Graph GQL syntax.
- Prefer SQL query rather than GQL query unless GQL is explicitly requested.
- GoogleSQL syntax is not PostgreSQL syntax.
- GQL requires output column names.
- GQL is neither GraphQL nor Cypher.
- If outputting GQL, double-check DDL definitions to ensure edge directions are correct.
- The output must be terminated with a semicolon.
- NULL_FILTERED indexes can be dropped using DROP INDEX, not DROP NULL_FILTERED INDEX.

Here is the database DDL:
` + "```\n" + strings.Join(resp.GetStatements(), ";\n") + ";\n```" + `

Here is the prototext of File Proto Descriptors:
` + "```\n" + prototext.Format(fds) + "```"
}

// geminiComposeQueryWithTools uses Gemini's function calling to dynamically
// fetch and search documentation via the docCache.
func geminiComposeQueryWithTools(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, project, location, model, s string, cache *docCache, hasAPI bool) (*output, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:     project,
		Location:    location,
		Backend:     genai.BackendVertexAI,
		HTTPOptions: genai.HTTPOptions{APIVersion: "v1"},
	})
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &fds); err != nil {
		return nil, err
	}

	basePrompt := geminiSystemPrompt(resp, &fds)
	toolPrompt := basePrompt + buildToolGuidance(cache, hasAPI)

	// Phase 1: Tool-use loop to gather documentation context
	history := []*genai.Content{
		{Role: genai.RoleUser, Parts: []*genai.Part{genai.NewPartFromText(s)}},
	}

	tools := buildToolDeclarations(hasAPI)

	for round := range maxToolCallRounds {
		result, err := client.Models.GenerateContent(ctx, model, history, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(toolPrompt)},
			},
			Tools: tools,
		})
		if err != nil {
			return nil, fmt.Errorf("tool-use round %d: %w", round, err)
		}

		functionCalls := result.FunctionCalls()
		if len(functionCalls) == 0 {
			slog.Debug("Tool-use phase complete", "rounds", round+1)
			break
		}

		if len(result.Candidates) > 0 {
			history = append(history, &genai.Content{
				Role:  "model",
				Parts: result.Candidates[0].Content.Parts,
			})
		}

		var responseParts []*genai.Part
		for _, fc := range functionCalls {
			slog.Debug("Gemini tool call", "function", fc.Name, "args", fc.Args)
			response := executeToolCall(ctx, fc, cache)
			responseParts = append(responseParts, genai.NewPartFromFunctionResponse(fc.Name, response))
		}

		history = append(history, &genai.Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	// Phase 2: Generate structured output using context from Phase 1
	return genaischema.GenerateObjectContent[*output](ctx, client, model,
		history,
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(basePrompt)},
			},
		})
}
