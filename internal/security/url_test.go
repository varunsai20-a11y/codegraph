package security

import (
	"testing"
)

func TestValidateGitHubURL_ValidInputs(t *testing.T) {
	tests := []struct {
		input             string
		expectedCanonical string
		expectedName      string
	}{
		{"https://github.com/gin-gonic/gin", "https://github.com/gin-gonic/gin", "gin"},
		{"https://github.com/gin-gonic/gin/", "https://github.com/gin-gonic/gin", "gin"},
		{"https://github.com/gin-gonic/gin.git", "https://github.com/gin-gonic/gin", "gin"},
		{"https://GITHUB.COM/Gin-Gonic/Gin.git/", "https://github.com/gin-gonic/gin", "Gin"},
		{"https://github.com/socketio/socket.io", "https://github.com/socketio/socket.io", "socket.io"},
		{"https://github.com/kubernetes/k8s.io", "https://github.com/kubernetes/k8s.io", "k8s.io"},
		{"https://github.com/facebook/react-router.v6", "https://github.com/facebook/react-router.v6", "react-router.v6"},
		{"https://github.com/a/b", "https://github.com/a/b", "b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			canonical, name, err := ValidateGitHubURL(tt.input)
			if err != nil {
				t.Fatalf("expected valid URL for '%s', got error: %v", tt.input, err)
			}
			if canonical != tt.expectedCanonical {
				t.Errorf("expected canonical '%s', got '%s'", tt.expectedCanonical, canonical)
			}
			if name != tt.expectedName {
				t.Errorf("expected repo name '%s', got '%s'", tt.expectedName, name)
			}
		})
	}
}

func TestValidateGitHubURL_InvalidInputs(t *testing.T) {
	invalidInputs := []string{
		"",
		"   ",
		"http://github.com/owner/repo",
		"ssh://git@github.com:owner/repo.git",
		"git@github.com:owner/repo.git",
		"https://evil.com/owner/repo",
		"https://user:pass@github.com/owner/repo",
		"https://github.com:8080/owner/repo",
		"https://github.com/owner",
		"https://github.com/owner/repo/blob/main/README.md",
		"https://github.com/owner/repo/pull/123",
		"https://github.com/owner/repo?ref=main",
		"https://github.com/owner/repo#readme",
		"https://github.com//owner/repo",
		"https://github.com/owner//repo",
		"https://github.com/owner/repo//",
		"https://github.com/owner/repo/../other",
		"https://github.com/owner_name/repo",      // Underscore in owner name invalid
		"https://github.com/-owner/repo",          // Leading hyphen in owner name invalid
		"https://github.com/owner-/repo",          // Trailing hyphen in owner name invalid
		"https://github.com/owner--name/repo",     // Double hyphen in owner name invalid
		"https://github.com/owner/repo%2fother",   // Percent-encoded slash invalid
		"https://github.com/owner/repo%2e%2e",     // Percent-encoded dots invalid
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			_, _, err := ValidateGitHubURL(input)
			if err == nil {
				t.Errorf("expected error for invalid input '%s', got nil", input)
			}
		})
	}
}

func TestCanonicalizeSourceURL(t *testing.T) {
	canonical, err := CanonicalizeSourceURL("https://GITHUB.COM/Gin-Gonic/Gin.git/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canonical != "https://github.com/gin-gonic/gin" {
		t.Errorf("expected canonicalized URL 'https://github.com/gin-gonic/gin', got '%s'", canonical)
	}
}
