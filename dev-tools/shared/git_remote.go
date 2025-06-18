package shared

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// GitRemoteInfo holds information parsed from git remotes
type GitRemoteInfo struct {
	Owner string
	Repo  string
	URL   string
	Name  string // Remote name (e.g., "origin")
}

// ParseGitRemotes parses git remotes to extract owner and repo information
func ParseGitRemotes() ([]GitRemoteInfo, error) {
	cmd := exec.Command("git", "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git remotes: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var remotes []GitRemoteInfo
	seen := make(map[string]bool) // To avoid duplicates from fetch/push URLs

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		remoteName := parts[0]
		remoteURL := parts[1]
		
		// Skip if we've already processed this remote
		key := remoteName + ":" + remoteURL
		if seen[key] {
			continue
		}
		seen[key] = true

		owner, repo, err := parseGitURL(remoteURL)
		if err != nil {
			continue // Skip unparseable URLs
		}

		remotes = append(remotes, GitRemoteInfo{
			Owner: owner,
			Repo:  repo,
			URL:   remoteURL,
			Name:  remoteName,
		})
	}

	return remotes, nil
}

// parseGitURL parses various git URL formats to extract owner and repo
func parseGitURL(url string) (owner, repo string, err error) {
	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")
	
	// Patterns for different URL formats:
	// - HTTPS: https://github.com/owner/repo
	// - SSH: git@github.com:owner/repo  
	// - Git protocol: git://github.com/owner/repo
	
	patterns := []string{
		// HTTPS format
		`https://[^/]+/([^/]+)/([^/]+)`,
		// SSH format
		`git@[^:]+:([^/]+)/([^/]+)`,
		// Git protocol
		`git://[^/]+/([^/]+)/([^/]+)`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if len(matches) == 3 {
			return matches[1], matches[2], nil
		}
	}
	
	return "", "", fmt.Errorf("unable to parse git URL: %s", url)
}

// GetDefaultRemote returns the default remote info, preferring "origin"
func GetDefaultRemote() (*GitRemoteInfo, error) {
	remotes, err := ParseGitRemotes()
	if err != nil {
		return nil, err
	}
	
	if len(remotes) == 0 {
		return nil, fmt.Errorf("no git remotes found")
	}
	
	// Prefer "origin" remote if it exists
	for _, remote := range remotes {
		if remote.Name == "origin" {
			return &remote, nil
		}
	}
	
	// Fallback to first remote
	return &remotes[0], nil
}

// GetOwnerRepo returns owner and repo from the default remote
func GetOwnerRepo() (owner, repo string, err error) {
	remote, err := GetDefaultRemote()
	if err != nil {
		return "", "", err
	}
	
	return remote.Owner, remote.Repo, nil
}


// GetRemotesByHost returns remotes grouped by host (e.g., github.com, gitlab.com)
func GetRemotesByHost() (map[string][]GitRemoteInfo, error) {
	remotes, err := ParseGitRemotes()
	if err != nil {
		return nil, err
	}
	
	hostGroups := make(map[string][]GitRemoteInfo)
	
	for _, remote := range remotes {
		host := extractHost(remote.URL)
		if host != "" {
			hostGroups[host] = append(hostGroups[host], remote)
		}
	}
	
	return hostGroups, nil
}

// extractHost extracts the host from a git URL
func extractHost(url string) string {
	// HTTPS format
	if strings.HasPrefix(url, "https://") {
		parts := strings.Split(strings.TrimPrefix(url, "https://"), "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	
	// SSH format
	if strings.Contains(url, "@") && strings.Contains(url, ":") {
		parts := strings.Split(url, "@")
		if len(parts) == 2 {
			hostPart := strings.Split(parts[1], ":")[0]
			return hostPart
		}
	}
	
	// Git protocol
	if strings.HasPrefix(url, "git://") {
		parts := strings.Split(strings.TrimPrefix(url, "git://"), "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	
	return ""
}

// IsGitRepository checks if the current directory is a git repository
func IsGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	err := cmd.Run()
	return err == nil
}

// GetCurrentBranch returns the current git branch name
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	
	return strings.TrimSpace(string(output)), nil
}

// GitRemoteConfig holds configuration for git remote detection
type GitRemoteConfig struct {
	PreferredRemotes []string // Preferred remote names in order (default: ["origin", "upstream"])
	AllowedHosts     []string // Allowed hosts (empty = allow all)
}

// DefaultGitRemoteConfig returns sensible defaults
func DefaultGitRemoteConfig() *GitRemoteConfig {
	return &GitRemoteConfig{
		PreferredRemotes: []string{"origin", "upstream"},
		AllowedHosts:     []string{"github.com", "gitlab.com"}, // Common Git hosts
		// No fallback - require git remote detection
	}
}

// GetOwnerRepoWithConfig returns owner/repo using configuration
func GetOwnerRepoWithConfig(config *GitRemoteConfig) (owner, repo string, err error) {
	if config == nil {
		config = DefaultGitRemoteConfig()
	}
	
	remotes, err := ParseGitRemotes()
	if err != nil {
		return "", "", fmt.Errorf("failed to parse git remotes: %w", err)
	}
	
	// Filter by allowed hosts if specified
	if len(config.AllowedHosts) > 0 {
		var filtered []GitRemoteInfo
		for _, remote := range remotes {
			host := extractHost(remote.URL)
			for _, allowedHost := range config.AllowedHosts {
				if host == allowedHost {
					filtered = append(filtered, remote)
					break
				}
			}
		}
		remotes = filtered
	}
	
	if len(remotes) == 0 {
		return "", "", fmt.Errorf("no suitable git remotes found")
	}
	
	// Try preferred remotes in order
	for _, preferred := range config.PreferredRemotes {
		for _, remote := range remotes {
			if remote.Name == preferred {
				return remote.Owner, remote.Repo, nil
			}
		}
	}
	
	// Fallback to first available remote
	return remotes[0].Owner, remotes[0].Repo, nil
}