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

	"github.com/MakeNowJust/heredoc"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ExecuteStatementArgs represents the arguments for the execute_statement tool
type ExecuteStatementArgs struct {
	Statement string `json:"statement" description:"Valid spanner-mycli statement. It can be SQL(Query, DML, DDL), GQL, and spanner-mycli client-side statements"`
}

// createMCPServer creates a new MCP server with the execute_statement tool
func createMCPServer(cli *Cli) *server.MCPServer {
	s := server.NewMCPServer("spanner-mycli", version,
		server.WithToolCapabilities(false))

	tool := mcp.NewTool("execute_statement",
		mcp.WithDescription(heredoc.Doc(
			`Execute any spanner-mycli statement.
If you want to check valid statements, see "HELP".
Result is ASCII table rendered, so you need to print as code block`)),
		mcp.WithString("statement",
			mcp.Required(),
			mcp.Description("Valid spanner-mycli statement. It can be SQL(Query, DML, DDL), GQL, and spanner-mycli client-side statements"),
		),
	)

	s.AddTool(tool, mcp.NewTypedToolHandler(func(ctx context.Context, request mcp.CallToolRequest, args ExecuteStatementArgs) (*mcp.CallToolResult, error) {
		statement := strings.TrimSuffix(args.Statement, ";")
		stmt, err := cli.parseStatement(&inputStatement{statement: statement, statementWithoutComments: statement, delim: ";"})
		if err != nil {
			return nil, err
		}

		var sb strings.Builder
		originalOutStream := cli.OutStream
		cli.OutStream = &sb
		defer func() { cli.OutStream = originalOutStream }()

		// Set Verbose to true to ensure result line is printed
		originalVerbose := cli.SystemVariables.Verbose
		cli.SystemVariables.Verbose = true
		defer func() { cli.SystemVariables.Verbose = originalVerbose }()

		_, err = cli.executeStatement(ctx, stmt, false, statement)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(sb.String()), nil
	}))

	return s
}

// RunMCP runs the MCP server
func (c *Cli) RunMCP(ctx context.Context) error {
	exists, err := c.Session.DatabaseExists()
	if err != nil {
		return err
	}

	if !exists {
		return NewExitCodeError(c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database)))
	}

	s := createMCPServer(c)

	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("failed to serve mcp: %w", err)
	}
	return nil
}
