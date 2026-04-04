import DOMPurify from 'dompurify';
import DOMPurify from 'dompurify';
import type { Task } from '../types';
import styles from './TaskArtifact.module.css';

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

  // Headings
  out = out.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  out = out.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  out = out.replace(/^# (.+)$/gm, '<h1>$1</h1>');

  // Bold
  out = out.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');

  // Inline code
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>');

  // Unordered lists
  out = out.replace(/^- (.+)$/gm, '<li>$1</li>');

  // Numbered lists
  out = out.replace(/^\d+\. (.+)$/gm, '<li>$1</li>');

  // Code blocks
  out = out.replace(/```[\w]*\n([\s\S]*?)```/g, '<pre class="code-block"><code>$1</code></pre>');

  // Paragraph breaks
  out = out.replace(/\n\n/g, '<br/><br/>');
  out = out.replace(/\n/g, '<br/>');

  return out;
}

function renderText(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br/>');
}

export function TaskArtifact({ task }: Props) {
  const result = task.result || '';

  return (
    <div className={styles.body}>
      {task.description && (
        <p className={styles.desc}>{task.description}</p>
      )}

      {!result ? (
        <p className={styles.empty}>No result yet</p>
      ) : isJSON(result) ? (
        <pre className={styles.pre}><code>{result}</code></pre>
      ) : isMarkdown(result) ? (
        <div
          className={styles.md}
          dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(renderMarkdown(result)) }}
        />
      ) : (
        <pre className={styles.pre}><code>{renderText(result)}</code></pre>
      )}
    </div>
  );
}
