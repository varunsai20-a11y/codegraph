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
	LangUnknown    Language = "UNKNOWN"
)

// ExtensionMap provides quick extension lookup.
var ExtensionMap = map[string]Language{
	".ts":   LangTypeScript,
	".tsx":  LangTSX,
	".js":   LangJavaScript,
	".jsx":  LangJSX,
	".mjs":  LangJavaScript,
	".cjs":  LangJavaScript,
	".py":   LangPython,
	".pyw":  LangPython,
	".go":   LangGo,
	".java": LangJava,
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
