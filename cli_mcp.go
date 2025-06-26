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
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"
)

// ExecuteStatementArgs represents the arguments for the execute_statement tool
type ExecuteStatementArgs struct {
	Statement string `json:"statement" description:"Valid spanner-mycli statement. It can be SQL(Query, DML, DDL), GQL, and spanner-mycli client-side statements"`
}

// executeStatementHandler handles the execute_statement tool
func executeStatementHandler(cli *Cli) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[ExecuteStatementArgs]) (*mcp.CallToolResultFor[struct{}], error) {
	return func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[ExecuteStatementArgs]) (*mcp.CallToolResultFor[struct{}], error) {
		// Parse the statement
		statement := strings.TrimSuffix(params.Arguments.Statement, ";")
		stmt, err := cli.parseStatement(&inputStatement{statement: statement, statementWithoutComments: statement, delim: ";"})
		if err != nil {
			return nil, err
		}

		// Create a string builder to capture the output
		var sb strings.Builder

		// Execute the statement with the string builder as the output
		_, err = cli.executeStatement(ctx, stmt, false, statement, &sb)
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil
	}
}

// createMCPServer creates a new MCP server with the execute_statement tool
func createMCPServer(cli *Cli) *mcp.Server {
	server := mcp.NewServer("spanner-mycli", version, nil)

	description := heredoc.Doc(
		`Execute any spanner-mycli statement.
If you want to check valid statements, see "HELP".
Result is ASCII table rendered, so you need to print as code block`)

	tool := mcp.NewServerTool("execute_statement", description, executeStatementHandler(cli),
		mcp.Input(
			mcp.Property("statement",
				mcp.Description("Valid spanner-mycli statement. It can be SQL(Query, DML, DDL), GQL, and spanner-mycli client-side statements"),
			),
		),
	)

	// Set tool annotations to provide hints about the tool's behavior
	tool.Tool.Annotations = &mcp.ToolAnnotations{
		Title:           "Execute Spanner SQL Statement",
		ReadOnlyHint:    false,               // Can modify the database
		DestructiveHint: lo.ToPtr(true),     // Can perform destructive operations
		IdempotentHint:  false,               // Repeated calls can have different effects
		OpenWorldHint:   lo.ToPtr(true),      // Interacts with external entities (the database)
	}

	server.AddTools(tool)

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
