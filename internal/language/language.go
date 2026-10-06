package language

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Language string

const (
	LangTypeScript Language = "TypeScript"
	LangJavaScript Language = "JavaScript"
	LangPython     Language = "Python"
	LangGo         Language = "Go"
	LangJava       Language = "Java"
	LangTSX        Language = "TSX"
	LangJSX        Language = "JSX"
	LangJSON       Language = "JSON"
	LangMarkdown   Language = "Markdown"
	LangYAML       Language = "YAML"
	LangTOML       Language = "TOML"
	LangSQL        Language = "SQL"
	LangShell      Language = "Shell"
	LangC          Language = "C"
	LangCPP        Language = "C++"
	LangRust       Language = "Rust"
	LangHTML       Language = "HTML"
	LangCSS        Language = "CSS"
	LangUnknown    Language = "UNKNOWN"
)

// ExtensionMap provides quick extension lookup.
var ExtensionMap = map[string]Language{
	".ts":    LangTypeScript,
	".mts":   LangTypeScript,
	".cts":   LangTypeScript,
	".tsx":   LangTSX,
	".js":    LangJavaScript,
	".mjs":   LangJavaScript,
	".cjs":   LangJavaScript,
	".jsx":   LangJSX,
	".astro": LangTypeScript,
	".py":    LangPython,
	".pyw":   LangPython,
	".go":    LangGo,
	".java":  LangJava,
	".json":  LangJSON,
	".jsonc": LangJSON,
	".md":    LangMarkdown,
	".mdx":   LangMarkdown,
	".yaml":  LangYAML,
	".yml":   LangYAML,
	".toml":  LangTOML,
	".sql":   LangSQL,
	".sh":    LangShell,
	".bash":  LangShell,
	".zsh":   LangShell,
	".c":     LangC,
	".h":     LangC,
	".cpp":   LangCPP,
	".hpp":   LangCPP,
	".cc":    LangCPP,
	".cxx":   LangCPP,
	".rs":    LangRust,
	".html":  LangHTML,
	".htm":   LangHTML,
	".css":   LangCSS,
	".scss":  LangCSS,
	".less":  LangCSS,
}

type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

// DetectLanguage determines language by file path extension and content inspection header.
func (d *Detector) DetectLanguage(filePath string) Language {
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, found := ExtensionMap[ext]; found {
		return lang
	}

	// Optional content header inspection for shebangs or package headers
	lang := d.detectFromHeader(filePath)
	return lang
}

func (d *Detector) detectFromHeader(filePath string) Language {
	file, err := os.Open(filePath)
	if err != nil {
		return LangUnknown
	}
	defer file.Close()

	reader := bufio.NewReader(io.LimitReader(file, 512))
	line, _, err := reader.ReadLine()
	if err != nil {
		return LangUnknown
	}

	lineStr := string(bytes.TrimSpace(line))
	if strings.HasPrefix(lineStr, "#!") {
		if strings.Contains(lineStr, "python") {
			return LangPython
		}
		if strings.Contains(lineStr, "node") {
			return LangJavaScript
		}
	}

	return LangUnknown
}
