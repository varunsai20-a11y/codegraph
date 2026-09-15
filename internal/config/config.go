package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               int
	MaxFileSize        int64
	WorkspaceRoot      string
	DatabasePath       string
	AllowedOrigins     []string
	DefaultExclusions  []string
	SupportedLanguages []string
}

func Load() *Config {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if val, err := strconv.Atoi(p); err == nil {
			port = val
		}
	}

	maxFileSize := int64(5 * 1024 * 1024) // 5 MB default
	if s := os.Getenv("MAX_FILE_SIZE"); s != "" {
		if val, err := strconv.ParseInt(s, 10, 64); err == nil {
			maxFileSize = val
		}
	}

	workspaceRoot := "./_workspaces"
	if w := os.Getenv("WORKSPACE_ROOT"); w != "" {
		workspaceRoot = w
	}

	dbPath := "codegraph.db"
	if db := os.Getenv("DATABASE_PATH"); db != "" {
		dbPath = db
	}

	allowedOrigins := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if origStr := os.Getenv("ALLOWED_ORIGINS"); origStr != "" {
		var custom []string
		for _, o := range strings.Split(origStr, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				custom = append(custom, trimmed)
			}
		}
		if len(custom) > 0 {
			allowedOrigins = custom
		}
	}

	return &Config{
		Port:               port,
		MaxFileSize:        maxFileSize,
		WorkspaceRoot:      workspaceRoot,
		DatabasePath:       dbPath,
		AllowedOrigins:     allowedOrigins,
		DefaultExclusions:  []string{".git", "node_modules", "dist", "build", "coverage", ".cache", ".tmp", "vendor"},
		SupportedLanguages: []string{"TypeScript", "JavaScript", "Python", "Go", "Java", "TSX", "JSX"},
	}
}
