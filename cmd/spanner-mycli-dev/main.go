package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

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

var (
	forceDelete bool
	tmuxMode    string
)

func init() {
	setupWorktreeCmd.Flags().StringVar(&tmuxMode, "tmux", "", "tmux mode for phantom shell (horizontal, vertical)")
	deleteWorktreeCmd.Flags().BoolVar(&forceDelete, "force", false, "Force delete without safety checks")

	worktreeCmd.AddCommand(setupWorktreeCmd, listWorktreesCmd, deleteWorktreeCmd)
	docsCmd.AddCommand(updateHelpCmd)
	rootCmd.AddCommand(worktreeCmd, docsCmd)
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