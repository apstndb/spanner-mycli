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

// Package llm contributes the GEMINI statement family (LLM-assisted query
// composition) to spanner-mycli through the Feature registration seam (issue
// #778). It is imported only by the full binary (via internal/mycli/feature/all);
// the core internal/mycli package does not import it, so google.golang.org/genai,
// github.com/apstndb/genaischema, github.com/apstndb/developerknowledge-go, and
// github.com/apstndb/spanner-docs-embed all stay off the core import path.
//
// GEMINI is the first feature to use the Flags/KongVars/ApplyFlags seam: it
// contributes the --vertexai-project/model/location CLI flags (Feature.Flags),
// the default-model/location help-template values (Feature.KongVars), and routes
// the parsed values into the CLI_VERTEXAI_* system variables through the registry
// (Feature.ApplyFlags), never by direct assignment.
package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/genaischema"
	"google.golang.org/genai"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// defaultVertexAIModel and defaultVertexAILocation are the built-in defaults for
// the GEMINI feature. They seed both the feature config and the
// --vertexai-model/location help text via Feature.KongVars.
const (
	defaultVertexAIModel    = "gemini-3.7-flash"
	defaultVertexAILocation = "global"

	genAIBackendEnterprise = "GEMINI_ENTERPRISE"
	genAIBackendGeminiAPI  = "GEMINI_API"

	thinkingLevelUnspecified = "UNSPECIFIED"
)

// docCacheStateKey namespaces the lazy Spanner-reference-doc cache in the
// per-Session feature store (mycli.FeatureState).
const docCacheStateKey = "llm.doccache"

// config holds the GEMINI feature's live CLI_GENAI_* and CLI_VERTEXAI_* variable
// state. Exactly one instance is allocated per Feature() call and
// captured by that Feature's Variable handlers, def handler closure, ApplyFlags
// closure, and constructed statements, so there is no package-level mutable state
// (session-isolation commitment, #778 §4.4).
type config struct {
	Backend       string // CLI_GENAI_BACKEND
	Project       string // CLI_VERTEXAI_PROJECT
	Model         string // CLI_VERTEXAI_MODEL
	Location      string // CLI_VERTEXAI_LOCATION
	ThinkingLevel string // CLI_GENAI_THINKING_LEVEL
}

// newConfig returns a config seeded with the built-in defaults, matching the
// pre-extraction newSystemVariablesWithDefaults seeding of Feature.VertexAIModel
// / VertexAILocation (VertexAIProject defaulted to empty).
func newConfig() *config {
	return &config{
		Backend:       genAIBackendEnterprise,
		Model:         defaultVertexAIModel,
		Location:      defaultVertexAILocation,
		ThinkingLevel: thinkingLevelUnspecified,
	}
}

// enumStringVar stores a canonical string while accepting case-insensitive
// values and aliases. It also exposes the canonical values to SET completion.
type enumStringVar struct {
	ptr     *string
	values  []string
	aliases map[string]string
}

func (v *enumStringVar) Get() (string, error) {
	return *v.ptr, nil
}

func (v *enumStringVar) Set(value string) error {
	normalized := strings.ToUpper(value)
	if canonical, ok := v.aliases[normalized]; ok {
		*v.ptr = canonical
		return nil
	}
	for _, valid := range v.values {
		if normalized == valid {
			*v.ptr = valid
			return nil
		}
	}
	return fmt.Errorf("invalid value %q, must be one of: %s", value, strings.Join(v.values, ", "))
}

func (v *enumStringVar) ValidValues() []string {
	values := make([]string, len(v.values))
	for i, value := range v.values {
		values[i] = "'" + value + "'"
	}
	return values
}

// flags is the kong-tagged flag struct contributed via Feature.Flags. The legacy
// --vertexai-* names remain available for configuration compatibility.
// VertexAIModel/Location are pointers to preserve "unset" semantics (an unset
// flag must not clobber a TOML/--set value).
type flags struct {
	VertexAIProject  string  `name:"vertexai-project" help:"Gemini Enterprise project override"`
	VertexAIModel    *string `name:"vertexai-model" help:"Gemini model (default: ${defaultVertexAIModel})"`
	VertexAILocation *string `name:"vertexai-location" help:"Gemini Enterprise location (default: ${defaultVertexAILocation})"`
}

// Feature returns the GEMINI Feature value. Each call allocates a FRESH config
// and a fresh flags struct, wiring fresh Variable/def/statement/ApplyFlags
// instances around them, so no state is shared between Feature values (#778 §4.4
// session-isolation commitment).
func Feature() mycli.Feature {
	cfg := newConfig()
	f := &flags{}
	return mycli.Feature{
		Name: "GEMINI",
		StatementDefs: []*mycli.StatementDef{
			{
				Descriptions: []mycli.StatementDescription{
					{
						Usage:  `Compose query using LLM`,
						Syntax: `GEMINI "<prompt>"`,
					},
				},
				Pattern: regexp.MustCompile(`(?is)^GEMINI\s+(?P<text>.*)$`),
				HandleGroups: func(groups map[string]string) (mycli.Statement, error) {
					return &GeminiStatement{Text: mycli.UnquoteString(groups["text"]), cfg: cfg}, nil
				},
			},
		},
		Vars: []mycli.FeatureVar{
			{
				Name: "CLI_GENAI_BACKEND",
				Desc: "GenAI backend: Gemini Enterprise Agent Platform or Gemini API.",
				Var: &enumStringVar{
					ptr:    &cfg.Backend,
					values: []string{genAIBackendEnterprise, genAIBackendGeminiAPI},
					aliases: map[string]string{
						"VERTEX_AI": genAIBackendEnterprise,
					},
				},
			},
			{
				Name: "CLI_GENAI_THINKING_LEVEL",
				Desc: "Gemini thinking level. UNSPECIFIED lets the model choose its default.",
				Var: &enumStringVar{
					ptr:    &cfg.ThinkingLevel,
					values: []string{thinkingLevelUnspecified, "MINIMAL", "LOW", "MEDIUM", "HIGH"},
				},
			},
			{
				Name: "CLI_VERTEXAI_PROJECT",
				Desc: "Gemini Enterprise project override. Defaults to CLI_PROJECT when empty.",
				Var:  mycli.StringVar(&cfg.Project),
			},
			{
				Name: "CLI_VERTEXAI_MODEL",
				Desc: "Gemini model for natural language features.",
				Var:  mycli.StringVar(&cfg.Model),
			},
			{
				Name: "CLI_VERTEXAI_LOCATION",
				Desc: "Gemini Enterprise location for natural language features.",
				Var:  mycli.StringVar(&cfg.Location),
			},
		},
		Flags: f,
		KongVars: map[string]string{
			"defaultVertexAIModel":    defaultVertexAIModel,
			"defaultVertexAILocation": defaultVertexAILocation,
		},
		// ApplyFlags routes parsed flag values into the CLI_VERTEXAI_* system
		// variables through the registry (set == systemVariables.SetFromSimple),
		// never by direct assignment. It runs before --set processing, so a --set
		// CLI_VERTEXAI_* overrides a flag, exactly as the pre-extraction direct
		// assignment (config.go) ordered flag application before applyFormatAndSetOptions.
		//
		// VertexAIProject mirrors the pre-extraction unconditional string assignment
		// (`sysVars.Feature.VertexAIProject = opts.VertexAIProject`); Model/Location
		// are applied only when explicitly provided (non-nil), preserving the
		// "unset flags must not clobber TOML/--set values" semantics.
		ApplyFlags: func(set func(name, value string) error) error {
			if err := set("CLI_VERTEXAI_PROJECT", f.VertexAIProject); err != nil {
				return err
			}
			if f.VertexAIModel != nil {
				if err := set("CLI_VERTEXAI_MODEL", *f.VertexAIModel); err != nil {
					return err
				}
			}
			if f.VertexAILocation != nil {
				if err := set("CLI_VERTEXAI_LOCATION", *f.VertexAILocation); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

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

// GeminiStatement composes a Cloud Spanner query from a natural-language prompt
// using Gemini. It implements no marker interfaces: like the pre-extraction
// in-core type, it is neither detached-compatible (blocked in detached session
// mode) nor READONLY-classified.
type GeminiStatement struct {
	Text string
	cfg  *config
}

func (s *GeminiStatement) Execute(ctx context.Context, session *mycli.Session) (*mycli.Result, error) {
	totalStart := time.Now()

	ddlStart := time.Now()
	resp, err := session.GetDatabaseDdlCached(ctx)
	if err != nil {
		return nil, err
	}
	slog.Debug("GEMINI timing: GetDatabaseDdlCached", "elapsed", time.Since(ddlStart))

	model := s.cfg.Model
	thinkingLevel := s.cfg.ThinkingLevel

	// Initialize session-scoped doc cache on first GEMINI call.
	cacheStart := time.Now()
	apiKey := devKnowledgeAPIKey()
	holder, err := mycli.FeatureState(ctx, session, docCacheStateKey,
		func(context.Context, *mycli.Session) (*docCacheHolder, error) {
			cache, err := buildDocCache(apiKey)
			if err != nil {
				return nil, err
			}
			return &docCacheHolder{cache: cache}, nil
		})
	if err != nil {
		return nil, err
	}
	cache := holder.cache
	slog.Debug("GEMINI timing: getOrCreateDocCache", "elapsed", time.Since(cacheStart))

	composeStart := time.Now()
	clientConfig := newGenAIClientConfig(s.cfg, session.ProjectID())
	composed, err := geminiComposeQueryWithTools(ctx, resp, clientConfig, model, thinkingLevel, s.Text, cache, apiKey != "")
	if err != nil {
		return nil, err
	}
	slog.Debug("GEMINI timing: geminiComposeQueryWithTools total", "elapsed", time.Since(composeStart))
	slog.Debug("GEMINI timing: Execute total", "elapsed", time.Since(totalStart))

	return resultFromComposedOutput(composed)
}

func resultFromComposedOutput(composed *output) (*mycli.Result, error) {
	if composed == nil {
		return nil, errors.New("GEMINI returned no response")
	}
	if composed.Statement == nil {
		if composed.ErrorDescription != "" {
			return nil, fmt.Errorf("GEMINI returned no statement: %s", composed.ErrorDescription)
		}
		return nil, errors.New("GEMINI returned no statement")
	}

	var rows []mycli.Row
	if composed.ErrorDescription != "" {
		rows = append(rows, mycli.NewRow("errorDescription", composed.ErrorDescription))
	}
	rows = append(rows,
		mycli.NewRow("text", composed.Statement.Text),
		mycli.NewRow("semanticDescription", composed.Statement.SemanticDescription),
		mycli.NewRow("syntaxDescription", composed.Statement.SyntaxDescription),
	)

	return &mycli.Result{
		PreInput:    composed.Statement.Text,
		Rows:        rows,
		TableHeader: mycli.NewTableHeader("Column", "Value"),
	}, nil
}

// docCacheHolder wraps the session-scoped docCache so it satisfies io.Closer
// (docCache.Close returns no error), letting the feature store close it at the
// end of Session.Close.
type docCacheHolder struct {
	cache *docCache
}

// Close releases the doc cache's zstd resources.
func (h *docCacheHolder) Close() error {
	h.cache.Close()
	return nil
}

// buildDocCache constructs the session-scoped doc cache, wiring the Developer
// Knowledge API searcher only when an API key is present. This reproduces the
// pre-extraction Session.getOrCreateDocCache body exactly; the FeatureState store
// guarantees it runs at most once per session (first GEMINI call wins, later
// calls reuse the cache regardless of the apiKey argument), matching the previous
// lazy-field semantics.
func buildDocCache(apiKey string) (*docCache, error) {
	var opts []docCacheOption
	if apiKey != "" {
		apiClient := newDevKnowledgeClient(apiKey)
		searcher := &devKnowledgeDocSearcher{client: apiClient}
		opts = append(opts,
			withDocFetcher(searcher.GetDocument),
			withDocBatchFetcher(searcher.BatchGetDocuments),
			withDocAPISearcher(searcher.Search),
		)
		slog.Debug("Developer Knowledge API enabled for documentation")
	} else {
		slog.Debug("No API key set, using embedded docs only")
	}

	cache, err := newDocCache(opts...)
	if err != nil {
		return nil, fmt.Errorf("create doc cache: %w", err)
	}

	if err := loadEmbeddedDocs(cache); err != nil {
		cache.Close()
		return nil, fmt.Errorf("load embedded docs: %w", err)
	}
	return cache, nil
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

// newGenAIClientConfig builds the backend-specific SDK configuration. Gemini
// API authentication is intentionally left to the SDK's GEMINI_API_KEY or
// GOOGLE_API_KEY environment-variable handling so secrets never become system
// variable values. The Enterprise backend uses the explicit project override,
// then the connected Spanner project, then the SDK's GOOGLE_CLOUD_PROJECT
// fallback.
func newGenAIClientConfig(cfg *config, sessionProject string) *genai.ClientConfig {
	clientConfig := &genai.ClientConfig{}
	if cfg.Backend == genAIBackendGeminiAPI {
		clientConfig.Backend = genai.BackendGeminiAPI
		return clientConfig
	}

	clientConfig.Backend = genai.BackendEnterprise
	clientConfig.Project = cmp.Or(cfg.Project, sessionProject)
	clientConfig.Location = cfg.Location
	clientConfig.HTTPOptions = genai.HTTPOptions{APIVersion: "v1"}
	return clientConfig
}

func newThinkingConfig(level string) *genai.ThinkingConfig {
	if level == thinkingLevelUnspecified {
		return nil
	}
	return &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevel(level)}
}

// geminiComposeQueryWithTools uses Gemini's function calling to dynamically
// fetch and search documentation via the docCache.
func geminiComposeQueryWithTools(ctx context.Context, resp *adminpb.GetDatabaseDdlResponse, clientConfig *genai.ClientConfig, model, thinkingLevel, s string, cache *docCache, hasAPI bool) (*output, error) {
	clientStart := time.Now()
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, err
	}
	slog.Debug("GEMINI timing: genai.NewClient", "elapsed", time.Since(clientStart))

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

	phase1Start := time.Now()
	for round := range maxToolCallRounds {
		roundStart := time.Now()
		result, err := client.Models.GenerateContent(ctx, model, history, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(toolPrompt)},
			},
			Tools:          tools,
			ThinkingConfig: newThinkingConfig(thinkingLevel),
		})
		if err != nil {
			return nil, fmt.Errorf("tool-use round %d: %w", round, err)
		}
		if result == nil {
			return nil, fmt.Errorf("tool-use round %d: GEMINI returned no response", round)
		}
		apiElapsed := time.Since(roundStart)

		functionCalls := result.FunctionCalls()
		if len(functionCalls) == 0 {
			slog.Debug("GEMINI timing: Phase 1 complete", "rounds", round+1, "total_elapsed", time.Since(phase1Start), "last_api_call", apiElapsed)
			break
		}

		if content := firstCandidateContent(result); content != nil {
			history = append(history, &genai.Content{
				Role:  genai.RoleModel,
				Parts: content.Parts,
			})
		}

		toolStart := time.Now()
		var responseParts []*genai.Part
		for _, fc := range functionCalls {
			slog.Debug("Gemini tool call", "function", fc.Name, "args", fc.Args)
			response := executeToolCall(ctx, fc, cache)
			responseParts = append(responseParts, genai.NewPartFromFunctionResponse(fc.Name, response))
		}
		slog.Debug("GEMINI timing: Phase 1 round", "round", round, "api_call", apiElapsed, "tool_exec", time.Since(toolStart), "functions", len(functionCalls))

		history = append(history, &genai.Content{
			Role:  genai.RoleUser,
			Parts: responseParts,
		})

		if round == maxToolCallRounds-1 {
			slog.Debug("GEMINI timing: Phase 1 exhausted all rounds", "rounds", maxToolCallRounds, "total_elapsed", time.Since(phase1Start))
		}
	}

	// Phase 2: Generate structured output using context from Phase 1
	phase2Start := time.Now()
	result, err := genaischema.GenerateObjectContent[*output](ctx, client, model,
		history,
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(basePrompt)},
			},
			ThinkingConfig: newThinkingConfig(thinkingLevel),
		})
	slog.Debug("GEMINI timing: Phase 2 (structured output)", "elapsed", time.Since(phase2Start))
	return result, err
}

func firstCandidateContent(result *genai.GenerateContentResponse) *genai.Content {
	if result == nil || len(result.Candidates) == 0 || result.Candidates[0] == nil {
		return nil
	}
	return result.Candidates[0].Content
}
