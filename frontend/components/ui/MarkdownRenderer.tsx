"use client";

import React from "react";

interface MarkdownRendererProps {
  content: string;
  className?: string;
  onCitationClick?: (citationLabel: string) => void;
}

export const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({
  content,
  className = "",
  onCitationClick,
}) => {
  if (!content) return null;

  // Split into code block blocks (```...```) and regular text blocks
  const parts = content.split(/(```[\s\S]*?```)/g);

  return (
    <div className={`space-y-2.5 font-sans leading-relaxed text-gray-200 ${className}`}>
      {parts.map((part, partIdx) => {
        if (!part) return null;

        // Code block handling
        if (part.startsWith("```") && part.endsWith("```")) {
          const lines = part.slice(3, -3).trim().split("\n");
          let language = "";
          let codeContent = lines.join("\n");
          if (lines.length > 0 && /^[a-zA-Z0-9_-]+$/.test(lines[0].trim())) {
            language = lines[0].trim();
            codeContent = lines.slice(1).join("\n");
          }

          return (
            <div key={partIdx} className="my-3 rounded-sm border border-border bg-background overflow-hidden font-mono text-xs">
              {language && (
                <div className="bg-surface px-3 py-1 text-[10px] text-gray-400 font-bold uppercase tracking-wider border-b border-border/60">
                  {language}
                </div>
              )}
              <pre className="p-3 text-cyan-200 overflow-x-auto leading-relaxed select-text">
                <code>{codeContent}</code>
              </pre>
            </div>
          );
        }

        // Regular text handling (paragraphs & headings)
        const paragraphs = part.split(/\n\s*\n/);
        return (
          <React.Fragment key={partIdx}>
            {paragraphs.map((para, pIdx) => {
              const trimmed = para.trim();
              if (!trimmed) return null;

              // Horizontal rule
              if (trimmed === "***" || trimmed === "---" || trimmed === "___") {
                return <hr key={pIdx} className="border-border/60 my-3" />;
              }

              // Heading 1 (# Header)
              if (/^#\s+/.test(trimmed)) {
                const title = trimmed.replace(/^#\s+/, "");
                return (
                  <h1 key={pIdx} className="text-sm font-bold font-mono text-cyan-200 uppercase tracking-wider mt-4 mb-2 border-b border-cyan-500/30 pb-1">
                    {renderInline(title, onCitationClick)}
                  </h1>
                );
              }

              // Heading 2 (## Header)
              if (/^##\s+/.test(trimmed)) {
                const title = trimmed.replace(/^##\s+/, "");
                return (
                  <h2 key={pIdx} className="text-xs font-bold font-mono text-cyan-300 uppercase tracking-wider mt-3 mb-1.5 border-b border-border/50 pb-1">
                    {renderInline(title, onCitationClick)}
                  </h2>
                );
              }

              // Heading 3 (### Header)
              if (/^###\s+/.test(trimmed)) {
                const title = trimmed.replace(/^###\s+/, "");
                return (
                  <h3 key={pIdx} className="text-xs font-bold font-mono text-cyan-400 uppercase tracking-wider mt-3 mb-1">
                    {renderInline(title, onCitationClick)}
                  </h3>
                );
              }

              // List items (- item or * item or 1. item)
              const lines = trimmed.split("\n");
              const isList = lines.every((line) => /^(\s*[-*]|\s*\d+\.)\s+/.test(line.trim()));
              if (isList) {
                return (
                  <ul key={pIdx} className="list-disc list-inside space-y-1 my-1.5 text-xs text-gray-200">
                    {lines.map((line, lIdx) => {
                      const cleanLine = line.trim().replace(/^(\s*[-*]|\s*\d+\.)\s+/, "");
                      return <li key={lIdx}>{renderInline(cleanLine, onCitationClick)}</li>;
                    })}
                  </ul>
                );
              }

              // Normal paragraph
              return (
                <p key={pIdx} className="text-xs leading-relaxed text-gray-200 my-1">
                  {renderInline(trimmed, onCitationClick)}
                </p>
              );
            })}
          </React.Fragment>
        );
      })}
    </div>
  );
};

// Render inline elements (bold, backticks, citations [E1], [E2])
function renderInline(text: string, onCitationClick?: (label: string) => void): React.ReactNode {
  // Regex pattern for citations ([E1], [E2]), backticks (`code`), and bold (**text**)
  const tokenRegex = /(\[E\d+\]|`[^`]+`|\*\*[^*]+\*\*)/g;
  const parts = text.split(tokenRegex);

  return parts.map((part, idx) => {
    if (!part) return null;

    // Citation badge ([E1], [E2])
    const citationMatch = part.match(/^\[(E\d+)\]$/);
    if (citationMatch) {
      const label = citationMatch[1];
      return (
        <span
          key={idx}
          onClick={() => onCitationClick && onCitationClick(label)}
          className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-cyan-950/80 text-cyan-300 border border-cyan-500/40 hover:bg-cyan-900 cursor-pointer mx-0.5 transition"
          title={`Jump to evidence ${label}`}
        >
          [{label}]
        </span>
      );
    }

    // Inline code (`code`)
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code
          key={idx}
          className="bg-cyan-950/60 text-cyan-300 border border-cyan-500/30 px-1.5 py-0.5 mx-0.5 rounded text-[11px] font-mono font-bold inline-block"
        >
          {part.slice(1, -1)}
        </code>
      );
    }

    // Bold text (**text**)
    if (part.startsWith("**") && part.endsWith("**")) {
      return (
        <strong key={idx} className="font-bold text-gray-100">
          {part.slice(2, -2)}
        </strong>
      );
    }

    return <span key={idx}>{part}</span>;
  });
}
