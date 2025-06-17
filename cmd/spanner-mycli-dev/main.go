package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "spanner-mycli-dev",
	Short: "Development tools for spanner-mycli project",
	Long: `spanner-mycli-dev provides project-specific development tools.

This tool focuses on spanner-mycli specific workflows:
- Phantom worktree management
- Documentation generation
- Project-specific automation`,
}

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Phantom worktree management",
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation generation tools",
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Project-specific review workflows",
}

var setupWorktreeCmd = &cobra.Command{
	Use:   "setup <worktree-name>",
	Short: "Setup phantom worktree with Claude configuration",
	Long: `Setup a phantom worktree based on origin/main with Claude settings.

This automates the workflow documented in CLAUDE.md:
1. Fetch latest changes from origin
2. Create phantom worktree based on origin/main  
3. Set up Claude settings symlink
4. Provide next steps for development

Examples:
  spanner-mycli-dev worktree setup issue-276-timeout-flag
  spanner-mycli-dev worktree setup fix-lint-warnings`,
	Args: cobra.ExactArgs(1),
	RunE: setupWorktree,
}

var listWorktreesCmd = &cobra.Command{
	Use:   "list",
	Short: "List existing phantom worktrees",
	RunE:  listWorktrees,
}

var deleteWorktreeCmd = &cobra.Command{
	Use:   "delete <worktree-name>",
	Short: "Delete phantom worktree with safety checks",
	Long: `Delete a phantom worktree with safety checks.

This command performs safety checks before deletion:
- Verifies no uncommitted changes exist
- Checks for untracked files that should be preserved
- Follows .gitignore deletion safety rules`,
	Args: cobra.ExactArgs(1),
	RunE: deleteWorktree,
}

var updateHelpCmd = &cobra.Command{
	Use:   "update-help",
	Short: "Generate help output for README.md",
	Long: `Generate help output files for updating README.md.

This creates formatted help output files in ./tmp/ directory:
- help_clean.txt: --help output for README.md
- statement_help.txt: --statement-help output for README.md

The files are ready for manual integration into README.md.`,
	RunE: updateHelp,
}

var geminiWorkflowCmd = &cobra.Command{
	Use:   "gemini <pr-number>",
	Short: "Complete Gemini Code Review workflow",
	Long: `Execute the complete Gemini Code Review workflow for spanner-mycli.

This command automates the project-specific Gemini review process with smart detection:
- Auto-detects if this is post-PR creation push (requests Gemini review)
- Always waits for review feedback (15 min timeout)
- Displays unresolved threads for manual handling

Usage scenarios:
1. After initial PR creation: Waits for automatic Gemini review
2. After pushes to existing PR: Requests /gemini review then waits

Use --force-request to always request review regardless of detection.

Designed for AI assistants to autonomously manage code review cycles.`,
	Args: cobra.ExactArgs(1),
	RunE: executeGeminiWorkflow,
}

var prWorkflowCmd = &cobra.Command{
	Use:   "pr-workflow",
	Short: "Complete PR creation and review workflow",
	Long: `Complete end-to-end PR workflow with explicit scenarios.

This command provides explicit control for different PR scenarios:

Subcommands:
  create - Create new PR from current branch
  review - Handle post-creation review cycles

This is more explicit than the auto-detecting 'gemini' command.`,
}

var createPRCmd = &cobra.Command{
	Use:   "create",
	Short: "Create new PR and wait for initial Gemini review",
	Long: `Create a new pull request and wait for the automatic Gemini review.

This command:
1. Creates PR using gh pr create with title/body from stdin or flags
2. Waits for automatic Gemini review (15 min timeout)
3. Displays any review threads for handling

For new PRs, Gemini automatically reviews so no manual trigger needed.`,
	RunE: createPRAndWait,
}

var reviewPRCmd = &cobra.Command{
	Use:   "review <pr-number>",
	Short: "Handle review cycle for existing PR after pushes",
	Long: `Handle review cycle for existing PR after making pushes.

This command:
1. Requests Gemini review (/gemini review)
2. Waits for review feedback (15 min timeout)  
3. Displays review threads for handling

Use this after pushing fixes/changes to an existing PR.`,
	Args: cobra.ExactArgs(1),
	RunE: handlePRReview,
}

var (
	forceDelete  bool
	tmuxMode     string
	forceRequest bool
	prTitle      string
	prBody       string
)

func init() {
	setupWorktreeCmd.Flags().StringVar(&tmuxMode, "tmux", "", "tmux mode for phantom shell (horizontal, vertical)")
	deleteWorktreeCmd.Flags().BoolVar(&forceDelete, "force", false, "Force delete without safety checks")
	geminiWorkflowCmd.Flags().BoolVar(&forceRequest, "force-request", false, "Always request Gemini review regardless of auto-detection")
	
	createPRCmd.Flags().StringVar(&prTitle, "title", "", "PR title (or use stdin)")
	createPRCmd.Flags().StringVar(&prBody, "body", "", "PR body (or use stdin)")

	worktreeCmd.AddCommand(setupWorktreeCmd, listWorktreesCmd, deleteWorktreeCmd)
	docsCmd.AddCommand(updateHelpCmd)
	reviewCmd.AddCommand(geminiWorkflowCmd)
	prWorkflowCmd.AddCommand(createPRCmd, reviewPRCmd)
	rootCmd.AddCommand(worktreeCmd, docsCmd, reviewCmd, prWorkflowCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupWorktree(cmd *cobra.Command, args []string) error {
	worktreeName := args[0]

	// Validate worktree name format
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(worktreeName) {
		return fmt.Errorf("invalid worktree name. Use only letters, numbers, hyphens, and underscores")
	}

	fmt.Printf("🔧 Creating phantom worktree: %s\n", worktreeName)

	// Check if phantom command exists
	if !commandExists("phantom") {
		return fmt.Errorf("phantom command not found. Please install phantom first:\n   https://github.com/aku11i/phantom")
	}

	// Check if worktree already exists
	if worktreeExists(worktreeName) {
		fmt.Printf("❌ Worktree '%s' already exists\n\n", worktreeName)
		fmt.Println("📋 Existing worktrees:")
		listWorktrees(cmd, []string{})
		return fmt.Errorf("worktree already exists")
	}

	// Fetch latest changes from origin
	fmt.Println("Fetching latest changes from origin...")
	if err := runCommand("git", "fetch", "origin"); err != nil {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}

	// Create phantom worktree based on origin/main with Claude settings
	fmt.Println("Creating worktree and setting up Claude configuration...")
	createCmd := []string{"phantom", "create", worktreeName, "--base", "origin/main", "--exec", "ln -sf ../../../../.claude .claude"}
	if err := runCommand(createCmd[0], createCmd[1:]...); err != nil {
		return fmt.Errorf("failed to create phantom worktree: %w", err)
	}

	fmt.Println("✅ Worktree created successfully!")
	fmt.Println()
	
	// Provide next steps
	fmt.Println("📋 Next steps:")
	shellCmd := fmt.Sprintf("phantom shell %s", worktreeName)
	if tmuxMode != "" {
		shellCmd += fmt.Sprintf(" --tmux-%s", tmuxMode)
	}
	fmt.Printf("   %s\n\n", shellCmd)
	
	fmt.Println("💡 Remember to:")
	fmt.Println("   - Record development insights in PR comments")
	fmt.Println("   - Run 'make test && make lint' before push")
	fmt.Printf("   - Check 'phantom exec %s git status' before cleanup\n\n", worktreeName)
	
	fmt.Println("🧹 Cleanup when done:")
	fmt.Printf("   phantom exec %s git status  # Verify no uncommitted changes\n", worktreeName)
	fmt.Printf("   phantom delete %s\n", worktreeName)

	return nil
}

func listWorktrees(cmd *cobra.Command, args []string) error {
	if !commandExists("phantom") {
		return fmt.Errorf("phantom command not found")
	}

	listCmd := exec.Command("phantom", "list")
	output, err := listCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		fmt.Println("No phantom worktrees found")
		return nil
	}

	fmt.Println("📋 Phantom worktrees:")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("   %s\n", line)
		}
	}

	return nil
}

func deleteWorktree(cmd *cobra.Command, args []string) error {
	worktreeName := args[0]

	if !commandExists("phantom") {
		return fmt.Errorf("phantom command not found")
	}

	if !worktreeExists(worktreeName) {
		return fmt.Errorf("worktree '%s' not found", worktreeName)
	}

	if !forceDelete {
		// Safety checks
		fmt.Printf("🔍 Performing safety checks for worktree: %s\n", worktreeName)
		
		// Check git status
		statusCmd := exec.Command("phantom", "exec", worktreeName, "git", "status", "--porcelain")
		statusOutput, err := statusCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to check git status: %w", err)
		}

		if len(statusOutput) > 0 {
			fmt.Println("❌ Uncommitted changes found:")
			fmt.Print(string(statusOutput))
			fmt.Println()
			fmt.Println("Please commit or stash changes before deletion, or use --force")
			return fmt.Errorf("uncommitted changes exist")
		}

		fmt.Println("✅ No uncommitted changes found")
	}

	// Delete the worktree
	fmt.Printf("🗑️  Deleting worktree: %s\n", worktreeName)
	if err := runCommand("phantom", "delete", worktreeName); err != nil {
		return fmt.Errorf("failed to delete worktree: %w", err)
	}

	fmt.Println("✅ Worktree deleted successfully")
	return nil
}

func updateHelp(cmd *cobra.Command, args []string) error {
	fmt.Println("📝 Updating help output for README.md...")

	// Create working directory
	if err := os.MkdirAll("./tmp", 0755); err != nil {
		return fmt.Errorf("failed to create tmp directory: %w", err)
	}

	// Generate --help output with proper column width
	fmt.Println("Generating --help output...")
	helpCmd := exec.Command("script", "-q", "./tmp/help_output.txt", "sh", "-c", "stty cols 200; go run . --help")
	if err := helpCmd.Run(); err != nil {
		return fmt.Errorf("failed to generate --help output: %w", err)
	}

	// Generate --statement-help output
	fmt.Println("Generating --statement-help output...")
	stmtCmd := exec.Command("go", "run", ".", "--statement-help")
	stmtOutput, err := stmtCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate --statement-help output: %w", err)
	}

	if err := os.WriteFile("./tmp/statement_help.txt", stmtOutput, 0644); err != nil {
		return fmt.Errorf("failed to write statement_help.txt: %w", err)
	}

	// Clean up help output format (remove first 2 characters from script output)
	fmt.Println("Cleaning up output format...")
	sedCmd := exec.Command("sed", "1s/^.\\{2\\}//", "./tmp/help_output.txt")
	cleanOutput, err := sedCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to clean help output: %w", err)
	}

	if err := os.WriteFile("./tmp/help_clean.txt", cleanOutput, 0644); err != nil {
		return fmt.Errorf("failed to write help_clean.txt: %w", err)
	}

	// Verify generated files exist and have content
	if !fileHasContent("./tmp/help_clean.txt") {
		return fmt.Errorf("generated help_clean.txt is empty")
	}

	if !fileHasContent("./tmp/statement_help.txt") {
		return fmt.Errorf("generated statement_help.txt is empty")
	}

	fmt.Println("✅ Help output files generated successfully in ./tmp/")
	fmt.Println()
	fmt.Println("📋 Generated files:")
	fmt.Println("   - help_clean.txt: --help output for README.md")
	fmt.Println("   - statement_help.txt: --statement-help output for README.md")
	fmt.Println()
	fmt.Println("⚠️  Manual step required:")
	fmt.Println("   Update README.md with generated content from the files above")
	fmt.Println()
	fmt.Println("💡 Future enhancement:")
	fmt.Println("   Consider adding automatic README.md update functionality")

	return nil
}

func executeGeminiWorkflow(cmd *cobra.Command, args []string) error {
	prNumber := args[0]
	
	fmt.Printf("🤖 Starting Gemini Code Review workflow for PR #%s...\n", prNumber)
	
	ghHelperPath := "./bin/gh-helper"
	if !fileExists(ghHelperPath) {
		ghHelperPath = "gh-helper"
	}
	
	shouldRequestReview := forceRequest
	
	// Auto-detect if we should request review (unless forced)
	if !forceRequest {
		fmt.Println("🔍 Auto-detecting if Gemini review request is needed...")
		
		// Check if there are recent commits after PR creation
		detected, err := detectNeedsGeminiRequest(prNumber)
		if err != nil {
			fmt.Printf("⚠️  Auto-detection failed: %v\n", err)
			fmt.Println("💡 Use --force-request to skip detection and always request review")
			return err
		}
		shouldRequestReview = detected
		
		if shouldRequestReview {
			fmt.Println("✅ Detected: This appears to be after pushes to existing PR")
		} else {
			fmt.Println("✅ Detected: This appears to be initial PR creation (Gemini auto-reviews)")
		}
	}
	
	// Step 1: Request Gemini review if needed
	if shouldRequestReview {
		fmt.Println("📝 Requesting Gemini code review...")
		if err := runCommand("gh", "pr", "comment", prNumber, "--body", "/gemini review"); err != nil {
			return fmt.Errorf("failed to request Gemini review: %w", err)
		}
		fmt.Println("✅ Gemini review requested")
	} else {
		fmt.Println("⏭️  Skipping review request (waiting for automatic Gemini review)")
	}
	
	// Step 2: Wait for review feedback using gh-helper
	fmt.Println("⏳ Waiting for Gemini review feedback (15 minute timeout)...")
	
	waitCmd := exec.Command(ghHelperPath, "reviews", "wait", prNumber, "--timeout", "15")
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr
	
	if err := waitCmd.Run(); err != nil {
		fmt.Printf("⚠️  Review wait completed with issues (this may be normal): %v\n", err)
	}
	
	// Step 3: Check for unresolved threads
	fmt.Println("\n🔍 Checking for review threads that need attention...")
	listCmd := exec.Command(ghHelperPath, "threads", "list", prNumber)
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	
	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("failed to list review threads: %w", err)
	}
	
	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Review feedback above\n")
	fmt.Printf("   2. Show details: %s threads show <THREAD_ID>\n", ghHelperPath)
	fmt.Printf("   3. Make fixes and push changes\n")
	fmt.Printf("   4. Reply: %s threads reply-commit <THREAD_ID> <COMMIT_HASH>\n", ghHelperPath)
	fmt.Printf("   5. Request follow-up: gh pr comment %s --body \"/gemini review\"\n", prNumber)
	
	return nil
}

func detectNeedsGeminiRequest(prNumber string) (bool, error) {
	// Strategy: Compare PR creation time with the latest commit time
	// If latest commit is significantly after PR creation, we likely need to request review
	
	// Get PR creation time and latest commit
	query := fmt.Sprintf(`
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    pullRequest(number: %s) {
      createdAt
      headRef {
        target {
          ... on Commit {
            committedDate
            history(first: 3) {
              nodes {
                committedDate
                message
              }
            }
          }
        }
      }
    }
  }
}`, prNumber)

	cmd := exec.Command("gh", "api", "graphql", "-F", "query="+query)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to fetch PR info: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					CreatedAt string `json:"createdAt"`
					HeadRef   struct {
						Target struct {
							CommittedDate string `json:"committedDate"`
							History       struct {
								Nodes []struct {
									CommittedDate string `json:"committedDate"`
									Message       string `json:"message"`
								} `json:"nodes"`
							} `json:"history"`
						} `json:"target"`
					} `json:"headRef"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return false, fmt.Errorf("failed to parse PR info: %w", err)
	}

	prCreatedAt := response.Data.Repository.PullRequest.CreatedAt
	latestCommitTime := response.Data.Repository.PullRequest.HeadRef.Target.CommittedDate

	// Parse times
	prTime, err := time.Parse(time.RFC3339, prCreatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse PR creation time: %w", err)
	}

	commitTime, err := time.Parse(time.RFC3339, latestCommitTime)
	if err != nil {
		return false, fmt.Errorf("failed to parse commit time: %w", err)
	}

	// If latest commit is more than 5 minutes after PR creation, assume we need to request review
	// This accounts for the time it might take to create the PR after the initial commits
	timeDiff := commitTime.Sub(prTime)
	needsRequest := timeDiff > 5*time.Minute

	fmt.Printf("   PR created: %s\n", prTime.Format("15:04:05"))
	fmt.Printf("   Latest commit: %s\n", commitTime.Format("15:04:05"))
	fmt.Printf("   Time difference: %v\n", timeDiff.Truncate(time.Second))

	// Also check recent commit messages for hints
	recentCommits := response.Data.Repository.PullRequest.HeadRef.Target.History.Nodes
	for _, commit := range recentCommits {
		commitMsg := strings.ToLower(commit.Message)
		// Look for fix/address keywords that suggest this is addressing review feedback
		if strings.Contains(commitMsg, "fix:") || 
		   strings.Contains(commitMsg, "address") || 
		   strings.Contains(commitMsg, "review") ||
		   strings.Contains(commitMsg, "feedback") {
			fmt.Printf("   Found review-related commit: %s\n", strings.Split(commit.Message, "\n")[0])
			needsRequest = true
			break
		}
	}

	return needsRequest, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func worktreeExists(name string) bool {
	cmd := exec.Command("phantom", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

func createPRAndWait(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 Creating new PR and waiting for automatic Gemini review...")
	
	// Build gh pr create command
	createArgs := []string{"pr", "create"}
	
	if prTitle != "" {
		createArgs = append(createArgs, "--title", prTitle)
	}
	
	if prBody != "" {
		createArgs = append(createArgs, "--body", prBody)
	}
	
	// If no flags provided, gh will prompt interactively or use stdin
	fmt.Println("📝 Creating pull request...")
	createCmd := exec.Command("gh", createArgs...)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	createCmd.Stdin = os.Stdin
	
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}
	
	fmt.Println("✅ PR created successfully!")
	
	// Get the PR number from the latest PR
	fmt.Println("🔍 Getting PR number...")
	getPRCmd := exec.Command("gh", "pr", "view", "--json", "number", "-q", ".number")
	output, err := getPRCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get PR number: %w", err)
	}
	
	prNumber := strings.TrimSpace(string(output))
	fmt.Printf("📋 PR Number: #%s\n", prNumber)
	
	// Wait for automatic Gemini review (no manual trigger needed)
	fmt.Println("⏳ Waiting for automatic Gemini review (15 minute timeout)...")
	
	ghHelperPath := "./bin/gh-helper"
	if !fileExists(ghHelperPath) {
		ghHelperPath = "gh-helper"
	}
	
	waitCmd := exec.Command(ghHelperPath, "reviews", "wait", prNumber, "--timeout", "15")
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr
	
	if err := waitCmd.Run(); err != nil {
		fmt.Printf("⚠️  Review wait completed with issues (this may be normal): %v\n", err)
	}
	
	// Check for review threads
	fmt.Println("\n🔍 Checking for review threads...")
	listCmd := exec.Command(ghHelperPath, "threads", "list", prNumber)
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	
	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("failed to list review threads: %w", err)
	}
	
	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Review feedback above\n")
	fmt.Printf("   2. Show details: %s threads show <THREAD_ID>\n", ghHelperPath)
	fmt.Printf("   3. Make fixes and push changes\n")
	fmt.Printf("   4. Use: spanner-mycli-dev pr-workflow review %s\n", prNumber)
	
	return nil
}

func handlePRReview(cmd *cobra.Command, args []string) error {
	prNumber := args[0]
	
	fmt.Printf("🔄 Handling review cycle for PR #%s...\n", prNumber)
	
	// Request Gemini review
	fmt.Println("📝 Requesting Gemini code review...")
	if err := runCommand("gh", "pr", "comment", prNumber, "--body", "/gemini review"); err != nil {
		return fmt.Errorf("failed to request Gemini review: %w", err)
	}
	fmt.Println("✅ Gemini review requested")
	
	// Wait for review feedback
	fmt.Println("⏳ Waiting for Gemini review feedback (15 minute timeout)...")
	
	ghHelperPath := "./bin/gh-helper"
	if !fileExists(ghHelperPath) {
		ghHelperPath = "gh-helper"
	}
	
	waitCmd := exec.Command(ghHelperPath, "reviews", "wait", prNumber, "--timeout", "15")
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr
	
	if err := waitCmd.Run(); err != nil {
		fmt.Printf("⚠️  Review wait completed with issues (this may be normal): %v\n", err)
	}
	
	// Check for review threads
	fmt.Println("\n🔍 Checking for review threads...")
	listCmd := exec.Command(ghHelperPath, "threads", "list", prNumber)
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	
	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("failed to list review threads: %w", err)
	}
	
	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Review feedback above\n")
	fmt.Printf("   2. Show details: %s threads show <THREAD_ID>\n", ghHelperPath)
	fmt.Printf("   3. Make fixes and push changes\n")
	fmt.Printf("   4. Reply: %s threads reply-commit <THREAD_ID> <COMMIT_HASH>\n", ghHelperPath)
	fmt.Printf("   5. Repeat: spanner-mycli-dev pr-workflow review %s\n", prNumber)
	
	return nil
}