//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"
)

// ExecuteStatementArgs represents the arguments for the execute_statement tool
type ExecuteStatementArgs struct {
	Statement string `json:"statement" jsonschema:"Valid spanner-mycli statement to execute"`
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}

// executeStatementHandler handles the execute_statement tool
func executeStatementHandler(cli *Cli) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[ExecuteStatementArgs]) (*mcp.CallToolResultFor[struct{}], error) {
	return func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[ExecuteStatementArgs]) (*mcp.CallToolResultFor[struct{}], error) {
		start := time.Now()

		// Log incoming request if debug is enabled
		if cli.SystemVariables.MCPDebug {
			slog.Debug("MCP request received",
				"method", "tools/call",
				"tool", "execute_statement",
				"statement", params.Arguments.Statement,
				"timestamp", start)
		}

		// Parse the statement
		statement := strings.TrimSuffix(params.Arguments.Statement, ";")
		stmt, err := cli.parseStatement(&inputStatement{statement: statement, statementWithoutComments: statement, delim: ";"})
		if err != nil {
			if cli.SystemVariables.MCPDebug {
				slog.Debug("MCP request failed",
					"error", err.Error(),
					"duration", time.Since(start))
			}
			return nil, err
		}

		// Create a string builder to capture the output
		var sb strings.Builder

		// Execute the statement with the string builder as the output
		_, err = cli.executeStatement(ctx, stmt, false, statement, &sb)
		if err != nil {
			if cli.SystemVariables.MCPDebug {
				slog.Debug("MCP execution failed",
					"error", err.Error(),
					"duration", time.Since(start))
			}
			return nil, err
		}

		result := &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}

		// Log response if debug is enabled
		if cli.SystemVariables.MCPDebug {
			slog.Debug("MCP response sent",
				"output_length", len(sb.String()),
				"duration", time.Since(start),
				"output_preview", truncateString(sb.String(), 100))
		}

		return result, nil
	}
}

// createMCPServer creates a new MCP server with the execute_statement tool
func createMCPServer(cli *Cli) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "spanner-mycli",
		Version: version,
	}, nil)

	description := heredoc.Doc(
		`Execute a spanner-mycli statement against the configured Spanner database.

Supports:
- SQL queries: SELECT, INSERT, UPDATE, DELETE
- DDL statements: CREATE TABLE/INDEX, ALTER TABLE, DROP TABLE/INDEX
- GQL queries for graph-based data
- Client-side commands: HELP, SHOW TABLES/COLUMNS/INDEX, SET variables, USE database

The result is returned as an ASCII-formatted table. When displaying in Markdown, wrap the output in a code block.

Use "HELP" command to see all available statements and their syntax.`)

	tool := &mcp.Tool{
		Name:        "execute_statement",
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Execute Spanner SQL Statement",
			ReadOnlyHint:    false,          // Can modify the database
			DestructiveHint: lo.ToPtr(true), // Can perform destructive operations
			IdempotentHint:  false,          // Repeated calls can have different effects
			OpenWorldHint:   lo.ToPtr(true), // Interacts with external entities (the database)
		},
	}

	mcp.AddTool(server, tool, executeStatementHandler(cli))

	return server
}

// RunMCP runs the MCP server
func (c *Cli) RunMCP(ctx context.Context) error {
	exists, err := c.SessionHandler.DatabaseExists()
	if err != nil {
		return err
	}

	if !exists {
		return NewExitCodeError(c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database)))
	}

	server := createMCPServer(c)

	transport := mcp.NewStdioTransport()
	session, err := server.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to serve mcp: %w", err)
	}
	// Wait for the session to complete
	return session.Wait()
}
