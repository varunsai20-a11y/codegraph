package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type LLMProviderConfig struct {
	Provider string
	Model    string
	APIKey   string
	Endpoint string
}

type Config struct {
	Port               int
	MaxFileSize        int64
	WorkspaceRoot      string
	DatabasePath       string
	AllowedOrigins     []string
	DefaultExclusions  []string
	SupportedLanguages []string
	LLM                LLMProviderConfig
	Gemini             LLMProviderConfig
	Groq               LLMProviderConfig
	LLMProviderOrder   []string
}

func loadEnvFile(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				if len(val) >= 2 {
					val = val[1 : len(val)-1]
				}
			}
			if os.Getenv(key) == "" && val != "" {
				os.Setenv(key, val)
			}
		}
	}
}

func Load() *Config {
	loadEnvFile(".env")

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

	allowedOrigins := []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:3001", "http://127.0.0.1:3001"}
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

	llmProv := strings.ToLower(os.Getenv("LLM_PROVIDER"))
	llmModel := os.Getenv("LLM_MODEL")
	llmKey := os.Getenv("LLM_API_KEY")
	llmEnd := os.Getenv("LLM_ENDPOINT")

	// Build Gemini config
	geminiKey := os.Getenv("GEMINI_API_KEY")
	geminiModel := os.Getenv("LLM_MODEL")
	if geminiModel == "" {
		geminiModel = "gemini-3.8-flash"
	}
	geminiEnd := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", geminiModel)
	geminiCfg := LLMProviderConfig{
		Provider: "gemini",
		Model:    geminiModel,
		APIKey:   geminiKey,
		Endpoint: geminiEnd,
	}

	// Build Groq config
	groqKey := os.Getenv("GROQ_API_KEY")
	groqModel := os.Getenv("GROQ_MODEL")
	if groqModel == "" {
		groqModel = "openai/gpt-oss-120b"
	}
	groqEnd := "https://api.groq.com/openai/v1/chat/completions"
	groqCfg := LLMProviderConfig{
		Provider: "groq",
		Model:    groqModel,
		APIKey:   groqKey,
		Endpoint: groqEnd,
	}

	// Build LLM Provider Order
	providerOrderStr := os.Getenv("LLM_PROVIDER_ORDER")
	var providerOrder []string
	if providerOrderStr != "" {
		for _, p := range strings.Split(providerOrderStr, ",") {
			trimmed := strings.ToLower(strings.TrimSpace(p))
			if trimmed != "" {
				providerOrder = append(providerOrder, trimmed)
			}
		}
	}
	if len(providerOrder) == 0 {
		providerOrder = []string{"gemini", "groq"}
	}

	if llmProv == "" {
		if geminiKey != "" {
			llmProv = "gemini"
			llmKey = geminiKey
			llmModel = geminiModel
			llmEnd = geminiEnd
		} else if groqKey != "" {
			llmProv = "groq"
			llmKey = groqKey
			llmModel = groqModel
			llmEnd = groqEnd
		} else if os.Getenv("OPENAI_API_KEY") != "" {
			llmProv = "openai"
			llmKey = os.Getenv("OPENAI_API_KEY")
			if llmModel == "" {
				llmModel = "gpt-4o"
			}
			if llmEnd == "" {
				llmEnd = "https://api.openai.com/v1/chat/completions"
			}
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
		LLM: LLMProviderConfig{
			Provider: llmProv,
			Model:    llmModel,
			APIKey:   llmKey,
			Endpoint: llmEnd,
		},
		Gemini:           geminiCfg,
		Groq:             groqCfg,
		LLMProviderOrder: providerOrder,
	}
}
