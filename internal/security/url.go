package security

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	githubOwnerCharsRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
	githubRepoRegex       = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// ValidateGitHubURL validates and canonicalizes a GitHub repository URL.
// It accepts only HTTPS github.com URLs with valid owner and repo names.
// Canonical format: https://github.com/<lowercased_owner>/<lowercased_repo>
func ValidateGitHubURL(rawURL string) (canonicalURL string, repoName string, err error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", "", fmt.Errorf("repository URL cannot be empty")
	}

	// 1. Reject query strings and fragments
	if strings.Contains(rawURL, "?") || strings.Contains(rawURL, "#") {
		return "", "", fmt.Errorf("URL query strings and fragments are not allowed")
	}

	// 2. Reject percent-encoded separators or traversal characters
	lowerRaw := strings.ToLower(rawURL)
	if strings.Contains(lowerRaw, "%2f") || strings.Contains(lowerRaw, "%2e") ||
		strings.Contains(lowerRaw, "%5c") || strings.Contains(lowerRaw, "%00") {
		return "", "", fmt.Errorf("percent-encoded path separators or traversal characters are not allowed")
	}

	// 3. Parse URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL structure: %w", err)
	}

	// 4. Scheme validation
	if strings.ToLower(parsed.Scheme) != "https" {
		return "", "", fmt.Errorf("scheme must be https")
	}

	// 5. Host validation
	if strings.ToLower(parsed.Hostname()) != "github.com" {
		return "", "", fmt.Errorf("host must be github.com")
	}

	// 6. User credentials and custom ports validation
	if parsed.User != nil {
		return "", "", fmt.Errorf("user credentials in URL are not allowed")
	}
	if parsed.Port() != "" {
		return "", "", fmt.Errorf("custom ports are not allowed")
	}

	// 7. Path-only repeated slash validation
	// Inspect parsed.Path to ensure path does not contain consecutive slashes //
	if strings.Contains(parsed.Path, "//") {
		return "", "", fmt.Errorf("malformed URL: consecutive slashes in path are prohibited")
	}

	// 8. Segment extraction before cleaning
	rawSegments := strings.Split(parsed.Path, "/")
	var segments []string
	for _, seg := range rawSegments {
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	// Reject path traversal segments . or ..
	for _, seg := range segments {
		if seg == "." || seg == ".." {
			return "", "", fmt.Errorf("path traversal segments '.' or '..' are not allowed")
		}
	}

	if len(segments) != 2 {
		return "", "", fmt.Errorf("repository URL must contain exactly an owner and repository name (e.g. https://github.com/owner/repo)")
	}

	owner := segments[0]
	repoSegment := segments[1]

	// 9. Strip terminal .git suffix strictly from repository segment
	if strings.HasSuffix(strings.ToLower(repoSegment), ".git") {
		repoSegment = repoSegment[:len(repoSegment)-4]
	}

	if repoSegment == "" || repoSegment == "." || repoSegment == ".." {
		return "", "", fmt.Errorf("repository name cannot be empty")
	}

	// 10. Validate owner according to GitHub username/org rules:
	// - Alphanumeric and single non-consecutive hyphens
	// - Cannot start or end with a hyphen
	// - Max 39 chars
	// - Underscores NOT allowed
	if len(owner) < 1 || len(owner) > 39 ||
		strings.HasPrefix(owner, "-") || strings.HasSuffix(owner, "-") ||
		strings.Contains(owner, "--") || !githubOwnerCharsRegex.MatchString(owner) {
		return "", "", fmt.Errorf("invalid GitHub owner name '%s': must contain only 1-39 alphanumeric characters or single non-consecutive hyphens", owner)
	}

	// 11. Validate repository name
	if !githubRepoRegex.MatchString(repoSegment) {
		return "", "", fmt.Errorf("invalid GitHub repository name '%s': contains unsupported characters", repoSegment)
	}

	canonical := fmt.Sprintf("https://github.com/%s/%s", strings.ToLower(owner), strings.ToLower(repoSegment))
	return canonical, repoSegment, nil
}

// CanonicalizeSourceURL attempts to produce a normalized canonical GitHub URL string,
// or returns the original trimmed rawURL if it is not a valid GitHub URL.
func CanonicalizeSourceURL(rawURL string) (string, error) {
	canonical, _, err := ValidateGitHubURL(rawURL)
	if err != nil {
		return "", err
	}
	return canonical, nil
}
