"use client";

import React from "react";

interface MarkdownRendererProps {
  content: string;
  className?: string;
  onCitationClick?: (citationLabel: string) => void;
}

type BlockType =
  | { type: "code"; language: string; content: string }
  | { type: "h1"; text: string }
  | { type: "h2"; text: string }
  | { type: "h3"; text: string }
  | { type: "h4"; text: string }
  | { type: "hr" }
  | { type: "ul"; items: string[] }
  | { type: "ol"; items: string[] }
  | { type: "diagram"; text: string }
  | { type: "p"; text: string };

export const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({
  content,
  className = "",
  onCitationClick,
}) => {
  if (!content) return null;

  const blocks = parseMarkdownBlocks(content);

  return (
    <div className={`space-y-3 font-sans text-xs leading-relaxed text-gray-200 ${className}`}>
      {blocks.map((block, idx) => {
        switch (block.type) {
          case "code":
            return (
              <div
                key={idx}
                className="my-3 rounded border border-border/80 bg-background/90 overflow-hidden font-mono text-xs shadow-sm"
              >
                {block.language && (
                  <div className="bg-surface/80 px-3 py-1 text-[10px] text-gray-400 font-bold uppercase tracking-wider border-b border-border/60">
                    {block.language}
                  </div>
                )}
                <pre className="p-3 text-cyan-200 overflow-x-auto leading-relaxed select-text font-mono text-[11px] whitespace-pre">
                  <code>{block.content}</code>
                </pre>
              </div>
            );

          case "h1":
            return (
              <h1
                key={idx}
                className="text-base font-bold font-sans text-gray-100 mt-5 mb-2 pb-1 border-b border-border/60"
              >
                {renderInline(block.text, onCitationClick)}
              </h1>
            );

          case "h2":
            return (
              <h2
                key={idx}
                className="text-sm font-bold font-sans text-cyan-300 mt-4 mb-2 pb-1 border-b border-border/40"
              >
                {renderInline(block.text, onCitationClick)}
              </h2>
            );

          case "h3":
            return (
              <h3
                key={idx}
                className="text-xs font-bold font-sans text-cyan-400 mt-4 mb-1.5 uppercase tracking-wide"
              >
                {renderInline(block.text, onCitationClick)}
              </h3>
            );

          case "h4":
            return (
              <h4
                key={idx}
                className="text-xs font-semibold font-sans text-gray-300 mt-3 mb-1"
              >
                {renderInline(block.text, onCitationClick)}
              </h4>
            );

          case "hr":
            return <hr key={idx} className="border-border/60 my-4" />;

          case "ul":
            return (
              <ul key={idx} className="list-disc pl-5 space-y-1.5 my-2 text-xs text-gray-200">
                {block.items.map((item, itemIdx) => (
                  <li key={itemIdx}>{renderInline(item, onCitationClick)}</li>
                ))}
              </ul>
            );

          case "ol":
            return (
              <ol key={idx} className="list-decimal pl-5 space-y-1.5 my-2 text-xs text-gray-200">
                {block.items.map((item, itemIdx) => (
                  <li key={itemIdx}>{renderInline(item, onCitationClick)}</li>
                ))}
              </ol>
            );

          case "diagram":
            return (
              <div
                key={idx}
                className="my-3 p-3 rounded border border-border/60 bg-background/80 overflow-x-auto font-mono text-[11px] text-cyan-200 whitespace-pre leading-relaxed shadow-inner"
              >
                {block.text}
              </div>
            );

          case "p":
            return (
              <p key={idx} className="text-xs leading-relaxed text-gray-200 my-2 whitespace-pre-wrap">
                {renderInline(block.text, onCitationClick)}
              </p>
            );

          default:
            return null;
        }
      })}
    </div>
  );
};

function parseMarkdownBlocks(content: string): BlockType[] {
  const blocks: BlockType[] = [];
  const rawParts = content.split(/(```[\s\S]*?```)/g);

  for (const part of rawParts) {
    if (!part) continue;

    if (part.startsWith("```") && part.endsWith("```")) {
      const lines = part.slice(3, -3).trim().split("\n");
      let language = "";
      let codeContent = lines.join("\n");
      if (lines.length > 0 && /^[a-zA-Z0-9_-]+$/.test(lines[0].trim())) {
        language = lines[0].trim();
        codeContent = lines.slice(1).join("\n");
      }
      blocks.push({ type: "code", language, content: codeContent });
      continue;
    }

    parseTextContent(part, blocks);
  }

  return blocks;
}

function parseTextContent(text: string, blocks: BlockType[]) {
  const chunks = text.split(/\n\s*\n/);

  for (let chunk of chunks) {
    chunk = chunk.trim();
    if (!chunk) continue;

    while (chunk.length > 0) {
      chunk = chunk.trim();
      if (!chunk) break;

      if (chunk === "***" || chunk === "---" || chunk === "___") {
        blocks.push({ type: "hr" });
        break;
      }

      const newlineIdx = chunk.indexOf("\n");
      const firstLine = newlineIdx !== -1 ? chunk.slice(0, newlineIdx).trim() : chunk.trim();

      if (/^#{1,4}\s+/.test(firstLine)) {
        if (firstLine.startsWith("# ")) {
          blocks.push({ type: "h1", text: firstLine.replace(/^#\s+/, "") });
        } else if (firstLine.startsWith("## ")) {
          blocks.push({ type: "h2", text: firstLine.replace(/^##\s+/, "") });
        } else if (firstLine.startsWith("### ")) {
          blocks.push({ type: "h3", text: firstLine.replace(/^###\s+/, "") });
        } else if (firstLine.startsWith("#### ")) {
          blocks.push({ type: "h4", text: firstLine.replace(/^####\s+/, "") });
        }

        if (newlineIdx !== -1) {
          chunk = chunk.slice(newlineIdx + 1);
          continue;
        } else {
          break;
        }
      }

      const lines = chunk.split("\n");
      const isList = lines.length > 0 && lines.every((l) => /^(\s*[-*+]|\s*\d+\.)\s+/.test(l.trim()));

      if (isList) {
        const isOrdered = /^\s*\d+\.\s+/.test(lines[0].trim());
        const items = lines.map((l) => l.trim().replace(/^(\s*[-*+]|\s*\d+\.)\s+/, ""));
        if (isOrdered) {
          blocks.push({ type: "ol", items });
        } else {
          blocks.push({ type: "ul", items });
        }
        break;
      }

      const isDiagram = chunk.includes("↓") || chunk.includes("→") || chunk.includes("-->") || chunk.includes("==>");
      if (isDiagram) {
        blocks.push({ type: "diagram", text: chunk });
        break;
      }

      blocks.push({ type: "p", text: chunk });
      break;
    }
  }
}

function renderInline(text: string, onCitationClick?: (label: string) => void): React.ReactNode {
  if (!text) return null;

  const tokenRegex = /(\[E\d+\]|`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/g;
  const parts = text.split(tokenRegex);

  return parts.map((part, idx) => {
    if (!part) return null;

    const citationMatch = part.match(/^\[(E\d+)\]$/);
    if (citationMatch) {
      const label = citationMatch[1];
      return (
        <span
          key={idx}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            if (onCitationClick) onCitationClick(label);
          }}
          className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-cyan-950/80 text-cyan-300 border border-cyan-500/40 hover:bg-cyan-900 cursor-pointer mx-1 transition select-none"
          title={`Jump to evidence ${label}`}
        >
          [{label}]
        </span>
      );
    }

    if (part.startsWith("`") && part.endsWith("`") && part.length >= 2) {
      const codeText = part.slice(1, -1);
      return (
        <code
          key={idx}
          className="bg-surface/90 text-cyan-200 border border-border/60 px-1.5 py-0.5 mx-0.5 rounded-sm text-[11px] font-mono font-medium"
        >
          {codeText}
        </code>
      );
    }

    if (part.startsWith("**") && part.endsWith("**") && part.length >= 4) {
      return (
        <strong key={idx} className="font-bold text-gray-100">
          {part.slice(2, -2)}
        </strong>
      );
    }

    if (part.startsWith("*") && part.endsWith("*") && part.length >= 2) {
      return (
        <em key={idx} className="italic text-gray-200">
          {part.slice(1, -1)}
        </em>
      );
    }

    return <span key={idx}>{part}</span>;
  });
}
