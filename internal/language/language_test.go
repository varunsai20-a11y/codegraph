package language

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	d := NewDetector()

	tests := []struct {
		path string
		want Language
	}{
		{"src/app.ts", LangTypeScript},
		{"components/Button.tsx", LangTSX},
		{"server/index.js", LangJavaScript},
		{"components/Header.jsx", LangJSX},
		{"scripts/build.py", LangPython},
		{"main.go", LangGo},
		{"App.java", LangJava},
		{"README.md", LangUnknown},
		{"Makefile", LangUnknown},
	}

	for _, tt := range tests {
		got := d.DetectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("DetectLanguage(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

func TestDetectFromHeader(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lang_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	script := filepath.Join(tmpDir, "myscript")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env python3\nprint('hello')\n"), 0755); err != nil {
		t.Fatal(err)
	}

	d := NewDetector()
	got := d.DetectLanguage(script)
	if got != LangPython {
		t.Errorf("expected Python for shebang script, got %v", got)
	}
}
