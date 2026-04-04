import type { Task } from '../types';

interface Props {
  task: Task;
}

/** Returns true if str looks like markdown */
function isMarkdown(str: string): boolean {
  return /^#|\*\*|- |\n- /.test(str);
}

/** Returns true if str looks like JSON */
function isJSON(str: string): boolean {
  const t = str.trim();
  return t.startsWith('{') || t.startsWith('[');
}

/** Very lightweight markdown → HTML-ish inline rendering */
function renderMarkdown(text: string): string {
  let out = text;

  // Headings: # → <strong> (avoid HTML injection in plain text render)
  out = out.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  out = out.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  out = out.replace(/^# (.+)$/gm, '<h1>$1</h1>');

  // Bold: **text** → <strong>text</strong>
  out = out.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');

  // Inline code: `code` → <code>code</code>
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>');

  // Unordered lists: lines starting with -
  out = out.replace(/^- (.+)$/gm, '<li>$1</li>');
  // wrap consecutive <li> lines in <ul>
  out = out.replace(/(<li>.*<\/li>)(?=\n(?!<li>))/gs, '$1');

  // Numbered lists
  out = out.replace(/^\d+\. (.+)$/gm, '<li>$1</li>');

  // Code blocks: ```...```
  out = out.replace(/```[\w]*\n([\s\S]*?)```/g, '<pre class="code-block"><code>$1</code></pre>');

  // Line breaks
  out = out.replace(/\n\n/g, '<br/><br/>');
  out = out.replace(/\n/g, '<br/>');

  return out;
}

function renderText(text: string): string {
  const escaped = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  return escaped.replace(/\n/g, '<br/>');
}

export function TaskArtifact({ task }: Props) {
  const desc = task.description || '';
  const result = task.result || '';

  return (
    <div className="artifact-body">
      {desc && <p className="artifact-desc">{desc}</p>}

      {!result ? (
        <p className="artifact-empty">No result yet</p>
      ) : isJSON(result) ? (
        <pre className="artifact-pre"><code>{result}</code></pre>
      ) : isMarkdown(result) ? (
        <div
          className="artifact-md"
          dangerouslySetInnerHTML={{ __html: renderMarkdown(result) }}
        />
      ) : (
        <pre className="artifact-pre"><code>{result}</code></pre>
      )}
    </div>
  );
}
