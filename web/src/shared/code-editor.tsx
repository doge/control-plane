import { useState } from "react";

export type CodeLanguage =
  | "plaintext"
  | "json"
  | "yaml"
  | "properties"
  | "javascript"
  | "typescript"
  | "java"
  | "shell"
  | "xml";

/** Edit file contents with syntax highlighting and accessible text editing. */
export function CodeEditor({
  value,
  onChange,
  language,
  ariaLabel = "Code contents",
  className = "",
}: {
  value: string;
  onChange: (value: string) => void;
  language: CodeLanguage;
  ariaLabel?: string;
  className?: string;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const [scrollLeft, setScrollLeft] = useState(0);
  const lines = value.split("\n");
  return (
    <div className={`file-code-editor ${className}`}>
      <div className="file-line-gutter" aria-hidden="true">
        <div style={{ transform: `translateY(-${scrollTop}px)` }}>
          {lines.map((_, index) => (
            <div key={index}>{index + 1}</div>
          ))}
        </div>
      </div>
      <div className="file-code-scroll">
        <div className="file-code-layer">
          <pre
            aria-hidden="true"
            style={{ transform: `translate(${-scrollLeft}px,${-scrollTop}px)` }}
          >
            {lines.map((line, index) => (
              <code
                key={index}
                dangerouslySetInnerHTML={{
                  __html: highlightCodeLine(line, language) || " ",
                }}
              />
            ))}
          </pre>
          <textarea
            aria-label={ariaLabel}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            onScroll={(event) => {
              setScrollTop(event.currentTarget.scrollTop);
              setScrollLeft(event.currentTarget.scrollLeft);
            }}
            spellCheck={false}
            wrap="off"
          />
        </div>
      </div>
    </div>
  );
}

/** Select an editor language from a file extension. */
export function detectSyntax(fileName: string): CodeLanguage {
  const ext = fileName.split(".").pop()?.toLowerCase();
  return ext === "json"
    ? "json"
    : ext === "yaml" || ext === "yml"
      ? "yaml"
      : ext === "properties" || ext === "conf"
        ? "properties"
        : ext === "js" || ext === "jsx"
          ? "javascript"
          : ext === "ts" || ext === "tsx"
            ? "typescript"
            : ext === "java"
              ? "java"
              : ext === "sh" || ext === "bash"
                ? "shell"
                : ext === "xml"
                  ? "xml"
                  : "plaintext";
}

function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function highlightCodeLine(line: string, language: CodeLanguage): string {
  const paint = (value: string, kind: string) =>
    `<span class="syntax-${kind}">${escapeHTML(value)}</span>`;
  let offset = 0,
    html = "";
  const token =
    /(<\/?[A-Za-z][^>]*>|\/\/.*$|#.*$|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b(?:true|false|null|undefined|yes|no|on|off)\b|-?\b\d+(?:\.\d+)?\b|\b(?:const|let|var|function|return|class|new|public|private|void|static|def|if|else|for|while|import|from|export|async|await|this|extends|package)\b|[{}\[\](),.:])/g;
  let match: RegExpExecArray | null;
  while ((match = token.exec(line))) {
    html += escapeHTML(line.slice(offset, match.index));
    const value = match[0];
    let kind = "punctuation";
    if (language === "xml" && value.startsWith("<")) kind = "tag";
    else if (value.startsWith("#") || value.startsWith("//")) {
      kind = "comment";
    } else if (
      value.startsWith('"') ||
      value.startsWith("'") ||
      value.startsWith("`")
    ) {
      kind = "string";
      if (
        language === "json" &&
        /^"/.test(value) &&
        line
          .slice(match.index + value.length)
          .trimStart()
          .startsWith(":")
      )
        kind = "key";
    } else if (/^-?\d/.test(value)) kind = "number";
    else if (/^(true|false|null|undefined|yes|no|on|off)$/.test(value)) {
      kind = "literal";
    } else if (
      /^(const|let|var|function|return|class|new|public|private|void|static|def|if|else|for|while|import|from|export|async|await|this|extends|package)$/.test(
        value,
      )
    )
      kind = "keyword";
    html += paint(value, kind);
    offset = match.index + value.length;
  }
  html += escapeHTML(line.slice(offset));
  if (language === "yaml") {
    const keyMatch = /^(\s*(?:-\s*)?)([^:#][^:]*?)(\s*:)/.exec(line);
    if (keyMatch) {
      return (
        escapeHTML(keyMatch[1]) +
        paint(keyMatch[2], "key") +
        paint(keyMatch[3], "punctuation") +
        highlightCodeLine(line.slice(keyMatch[0].length), "plaintext")
      );
    }
  }
  if (language === "properties") {
    const keyMatch = /^(\s*)([^#!=\s][^=:]*)(\s*[=:])/.exec(line);
    if (keyMatch) {
      return (
        escapeHTML(keyMatch[1]) +
        paint(keyMatch[2], "key") +
        paint(keyMatch[3], "punctuation") +
        highlightCodeLine(line.slice(keyMatch[0].length), "plaintext")
      );
    }
  }
  return html || "&nbsp;";
}
