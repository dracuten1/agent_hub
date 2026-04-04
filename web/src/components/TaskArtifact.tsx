import type { Task } from '../types';
import styles from './TaskArtifact.module.css';

interface Props {
  task: Task;
}

// ─── Safe markdown-to-React renderer (no dangerouslySetInnerHTML) ──────────────

/** Escape HTML entities */
function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/** Render **bold** segments as React nodes */
function renderBoldReact(text: string): React.ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return parts.map((part, i) => {
    if (/^\*\*.+\*\*$/.test(part)) {
      return <strong key={i}>{part.slice(2, -2)}</strong>;
    }
    return <span key={i}>{esc(part)}</span>;
  });
}

type InlineTok =
  | { kind: 'h1'; text: string }
  | { kind: 'h2'; text: string }
  | { kind: 'h3'; text: string }
  | { kind: 'li'; text: string }
  | { kind: 'blank' }
  | { kind: 'para'; text: string };

function parseInline(text: string): InlineTok[] {
  const lines = text.split('\n');
  const tokens: InlineTok[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line === '') { tokens.push({ kind: 'blank' }); i++; continue; }
    if (/^#{3}\s/.test(line)) { tokens.push({ kind: 'h3', text: line.replace(/^#{3}\s/, '') }); i++; continue; }
    if (/^#{2}\s/.test(line)) { tokens.push({ kind: 'h2', text: line.replace(/^#{2}\s/, '') }); i++; continue; }
    if (/^#\s/.test(line))    { tokens.push({ kind: 'h1', text: line.replace(/^#\s/, '') }); i++; continue; }
    if (/^-\s/.test(line))    { tokens.push({ kind: 'li', text: line.replace(/^-\s/, '') }); i++; continue; }
    const paraLines: string[] = [];
    while (i < lines.length && lines[i] !== '' && !/^#/.test(lines[i]) && !/^-\s/.test(lines[i])) {
      paraLines.push(lines[i]); i++;
    }
    if (paraLines.length) tokens.push({ kind: 'para', text: paraLines.join(' ') });
  }
  return tokens;
}

/** Split result into code-block and non-code sections */
function tokenizeCode(text: string): Array<{ kind: 'text' | 'code'; content: string }> {
  const parts: Array<{ kind: 'text' | 'code'; content: string }> = [];
  const re = /```[\w]*\n?([\s\S]*?)```/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push({ kind: 'text', content: text.slice(last, m.index) });
    parts.push({ kind: 'code', content: m[1] });
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push({ kind: 'text', content: text.slice(last) });
  return parts.length > 0 ? parts : [{ kind: 'text', content: text }];
}

function isMarkdown(s: string): boolean {
  return /^#|\*\*|- |\n- /.test(s);
}

function isJSON(s: string): boolean {
  const t = s.trim();
  return t.startsWith('{') || t.startsWith('[');
}

function renderResult(result: string): React.ReactNode {
  if (!result) return null;

  // JSON — pretty-print in <pre>
  if (isJSON(result)) {
    let formatted: string;
    try { formatted = JSON.stringify(JSON.parse(result), null, 2); }
    catch { formatted = result; }
    return <pre className={styles.pre}><code>{formatted}</code></pre>;
  }

  // Plain text — no markdown
  if (!isMarkdown(result)) {
    return (
      <pre className={styles.pre}>
        <code>
          {result.split('\n').map((line, i) => (
            <span key={i}>{esc(line)}{i < result.split('\n').length - 1 ? <br/> : null}</span>
          ))}
        </code>
      </pre>
    );
  }

  // Markdown — React-safe rendering, no dangerouslySetInnerHTML
  return tokenizeCode(result).map((part, pi) =>
    part.kind === 'code'
      ? <pre key={pi} className={styles.codeBlock}><code>{esc(part.content)}</code></pre>
      : <div key={pi}>
          {parseInline(part.content).map((tok, ti) => {
            if (tok.kind === 'blank') return <br key={ti} />;
            if (tok.kind === 'h1') return <h2 key={ti} className={styles.mdH1}>{tok.text}</h2>;
            if (tok.kind === 'h2') return <h3 key={ti} className={styles.mdH2}>{tok.text}</h3>;
            if (tok.kind === 'h3') return <h4 key={ti} className={styles.mdH3}>{tok.text}</h4>;
            if (tok.kind === 'li') return <li key={ti} className={styles.mdLi}>{renderBoldReact(tok.text)}</li>;
            return <p key={ti} className={styles.mdPara}>{renderBoldReact(tok.text)}</p>;
          })}
        </div>
  );
}

export function TaskArtifact({ task }: Props) {
  return (
    <div className={styles.body}>
      {task.description && (
        <p className={styles.desc}>{task.description}</p>
      )}
      {!task.result ? (
        <p className={styles.empty}>No result yet</p>
      ) : (
        renderResult(task.result)
      )}
    </div>
  );
}
