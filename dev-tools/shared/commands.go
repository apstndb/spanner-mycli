package shared

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Command creation patterns to eliminate duplication across dev-tools.
// These factories provide consistent command setup with standardized flags and behavior.

// CommandType represents different types of commands for appropriate flag assignment
type CommandType int

const (
	OperationalCommand CommandType = iota // Commands that perform operations (reviews, threads)
	WorkflowCommand                       // Commands that manage workflows (worktree, docs)
	UtilityCommand                        // Commands that provide utilities (help, version)
)

// StandardFlags represents common flag patterns
type StandardFlags struct {
	Owner     string
	Repo      string
	Timeout   string
	JSON      bool
	Message   string
	Mention   string
	Verbose   bool
	DryRun    bool
}

// CommandConfig holds configuration for command creation
type CommandConfig struct {
	Use         string
	Short       string
	Long        string
	Type        CommandType
	Args        cobra.PositionalArgs
	RunE        func(*cobra.Command, []string) error
	Flags       []string // Flag names to add: "owner", "repo", "timeout", "json", etc.
	Aliases     []string
	Hidden      bool
	Deprecated  string
}

// NewOperationalCommand creates a command optimized for GitHub operations
func NewOperationalCommand(use, short, long string, runE func(*cobra.Command, []string) error) *cobra.Command {
	cmd := NewCommandWithConfig(CommandConfig{
		Use:   use,
		Short: short,
		Long:  long,
		Type:  OperationalCommand,
		RunE:  runE,
		Flags: []string{"owner", "repo", "timeout", "json"}, // Standard operational flags
	})
	
	// Operational commands silence usage help since most errors are runtime issues
	cmd.SilenceUsage = true
	
	return cmd
}

// NewWorkflowCommand creates a command for workflow management
func NewWorkflowCommand(use, short, long string, runE func(*cobra.Command, []string) error) *cobra.Command {
	return NewCommandWithConfig(CommandConfig{
		Use:   use,
		Short: short,
		Long:  long,
		Type:  WorkflowCommand,
		RunE:  runE,
		Flags: []string{"verbose", "dry-run"}, // Standard workflow flags
	})
}

// NewCommandWithConfig creates a command with detailed configuration
func NewCommandWithConfig(config CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:        config.Use,
		Short:      config.Short,
		Long:       config.Long,
		Args:       config.Args,
		RunE:       config.RunE,
		Aliases:    config.Aliases,
		Hidden:     config.Hidden,
		Deprecated: config.Deprecated,
	}

	// Add specified flags
	AddFlags(cmd, config.Flags...)

	return cmd
}

// AddFlags adds specified flags to a command
func AddFlags(cmd *cobra.Command, flagNames ...string) {
	for _, flagName := range flagNames {
		switch flagName {
		case "owner":
			cmd.Flags().StringVar(&GlobalFlags.Owner, "owner", DefaultOwner, "GitHub repository owner")
		case "repo":
			cmd.Flags().StringVar(&GlobalFlags.Repo, "repo", DefaultRepo, "GitHub repository name")
		case "timeout":
			cmd.Flags().StringVar(&GlobalFlags.Timeout, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s, 15m)")
		case "json":
			cmd.Flags().BoolVar(&GlobalFlags.JSON, "json", false, "Output structured JSON for programmatic use")
		case "message":
			cmd.Flags().StringVar(&GlobalFlags.Message, "message", "", "Message text (or use stdin)")
		case "mention":
			cmd.Flags().StringVar(&GlobalFlags.Mention, "mention", "", "Username to mention (without @)")
		case "verbose":
			cmd.Flags().BoolVarP(&GlobalFlags.Verbose, "verbose", "v", false, "Enable verbose output")
		case "dry-run":
			cmd.Flags().BoolVar(&GlobalFlags.DryRun, "dry-run", false, "Show what would be done without executing")
		default:
			fmt.Fprintf(os.Stderr, "Warning: Unknown flag type '%s'\n", flagName)
		}
	}
}

// AddPersistentFlags adds flags that are inherited by all subcommands
func AddPersistentFlags(cmd *cobra.Command, flagNames ...string) {
	for _, flagName := range flagNames {
		switch flagName {
		case "owner":
			cmd.PersistentFlags().StringVar(&GlobalFlags.Owner, "owner", DefaultOwner, "GitHub repository owner")
		case "repo":
			cmd.PersistentFlags().StringVar(&GlobalFlags.Repo, "repo", DefaultRepo, "GitHub repository name")
		case "timeout":
			cmd.PersistentFlags().StringVar(&GlobalFlags.Timeout, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s, 15m)")
		case "json":
			cmd.PersistentFlags().BoolVar(&GlobalFlags.JSON, "json", false, "Output structured JSON for programmatic use")
		case "verbose":
			cmd.PersistentFlags().BoolVarP(&GlobalFlags.Verbose, "verbose", "v", false, "Enable verbose output")
		default:
			fmt.Fprintf(os.Stderr, "Warning: Unknown persistent flag type '%s'\n", flagName)
		}
	}
}

// GlobalFlags holds shared flag values across commands
var GlobalFlags = &StandardFlags{}

// DefaultOwner and DefaultRepo are dynamically initialized from git remotes
var (
	DefaultOwner string
	DefaultRepo  string
)

func init() {
	// Initialize defaults from git remote during package initialization
	// This happens automatically when the shared package is imported
	if err := initializeDefaults(); err != nil {
		// Log error but don't panic during package initialization
		// Applications can call InitializeDefaults() explicitly if needed
		DefaultOwner = ""
		DefaultRepo = ""
	}
}

// initializeDefaults sets up default values from git remote
// Returns error instead of panicking for better error handling
//
// CRITICAL FIX (Issue #306 review): Changed from panic() to structured error
// to prevent CLI tools from crashing during package initialization
func initializeDefaults() error {
	owner, repo, err := GetOwnerRepo()
	if err != nil {
		return fmt.Errorf("failed to detect git remote: %w. Please ensure you're in a git repository with remotes configured", err)
	}
	DefaultOwner = owner
	DefaultRepo = repo
	return nil
}

// InitializeDefaults sets up default values from git remote (public API for manual initialization)
func InitializeDefaults() error {
	return initializeDefaults()
}

// InitializeDefaultsWithConfig sets up defaults using configuration
func InitializeDefaultsWithConfig(config *GitRemoteConfig) {
	if config == nil {
		config = DefaultGitRemoteConfig()
	}
	
	owner, repo, err := GetOwnerRepoWithConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to detect git remote with config: %v", err))
	}
	DefaultOwner = owner
	DefaultRepo = repo
}


// Timeout parsing helper
func ParseTimeoutFlag() (time.Duration, error) {
	if GlobalFlags.Timeout == "" {
		return 5 * time.Minute, nil // Default timeout
	}
	return time.ParseDuration(GlobalFlags.Timeout)
}

// Command helper functions

// SetupRootCommand configures a root command with standard persistent flags
func SetupRootCommand(cmd *cobra.Command, persistentFlags ...string) {
	if len(persistentFlags) == 0 {
		// Default persistent flags for most dev tools
		persistentFlags = []string{"owner", "repo", "timeout", "json"}
	}
	AddPersistentFlags(cmd, persistentFlags...)
}

// AddSubcommands adds multiple subcommands to a parent command
func AddSubcommands(parent *cobra.Command, children ...*cobra.Command) {
	for _, child := range children {
		parent.AddCommand(child)
	}
}

// CreateCommandGroup creates a command group with consistent setup
func CreateCommandGroup(name, short, long string, commands ...*cobra.Command) *cobra.Command {
	group := &cobra.Command{
		Use:   name,
		Short: short,
		Long:  long,
	}
	
	AddSubcommands(group, commands...)
	return group
}