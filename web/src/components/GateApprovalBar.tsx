import { useState } from 'react';
import { ChevronDown } from 'lucide-react';
import styles from './GateApprovalBar.module.css';

interface TaskSummary {
  title: string;
  status: string;
  result?: string;
  assignee?: string;
}

interface Props {
  gatePhaseIndex: number;
  previousPhase: {
    name: string;
    tasks: TaskSummary[];
    total: number;
    done: number;
  } | null;
  onApprove: (note: string) => void;
  onReject: (note: string) => void;
  approveLoading: boolean;
  rejectLoading: boolean;
  error: string;
}

function StatusPill({ status }: { status: string }) {
  const map: Record<string, string> = {
    done:         styles.done,
    completed:    styles.done,
    in_progress:  styles.running,
    failed:       styles.failed,
    blocked:      styles.blocked,
    available:    styles.muted,
  };
  return (
    <span className={`${styles.pill} ${map[status] ?? styles.muted}`}>
      {status.replace(/_/g, ' ')}
    </span>
  );
}

function ResultPreview({ result }: { result?: string }) {
  if (!result) return <span className={styles.previewNone}>—</span>;
  const snippet = result.length > 120 ? result.slice(0, 120) + '…' : result;
  return <span className={styles.previewSnippet}>{snippet}</span>;
}

export function GateApprovalBar({
  gatePhaseIndex,
  previousPhase,
  onApprove,
  onReject,
  approveLoading,
  rejectLoading,
  error,
}: Props) {
  const [note, setNote] = useState('');
  const [prevExpanded, setPrevExpanded] = useState(false);
  const busy = approveLoading || rejectLoading;

  function submitApprove() {
    onApprove(note);
    setNote('');
  }

  function submitReject() {
    onReject(note);
    setNote('');
  }

  return (
    <div className={styles.bar}>
      {/* Header */}
      <div className={styles.header}>
        <div className={styles.gatePill}>Gate · Phase {gatePhaseIndex}</div>
        <h3 className={styles.title}>Approval required</h3>
        {previousPhase && (
          <p className={styles.summary}>
            <strong>{previousPhase.done}/{previousPhase.total}</strong> tasks completed
            {' '}in{' '}
            <button
              className={styles.phaseLink}
              onClick={() => setPrevExpanded(e => !e)}
            >
              {previousPhase.name}
              <ChevronDown
                size={12}
                className={`${styles.chevron} ${prevExpanded ? styles.chevronOpen : ''}`}
              />
            </button>
          </p>
        )}
      </div>

      {/* Previous phase task results — collapsed by default */}
      {previousPhase && previousPhase.tasks.length > 0 && (
        <div className={`${styles.prevResults} ${prevExpanded ? styles.prevResultsOpen : ''}`}>
          {previousPhase.tasks.map((task, i) => (
            <div key={i} className={styles.taskRow}>
              <StatusPill status={task.status} />
              <span className={styles.taskTitle}>{task.title}</span>
              {task.assignee && <span className={styles.assignee}>{task.assignee}</span>}
              <ResultPreview result={task.result} />
            </div>
          ))}
        </div>
      )}

      {/* Note input */}
      <div className={styles.noteWrap}>
        <label className={styles.noteLabel} htmlFor="gate-note">
          Note <span className={styles.noteOptional}>(optional)</span>
        </label>
        <textarea
          id="gate-note"
          className={styles.noteInput}
          placeholder="Add a note for the record…"
          value={note}
          onChange={e => setNote(e.target.value)}
          rows={2}
          disabled={busy}
        />
      </div>

      {/* Error */}
      {error && <p className={styles.error}>{error}</p>}

      {/* Actions */}
      <div className={styles.actions}>
        <button
          className={styles.rejectBtn}
          onClick={submitReject}
          disabled={busy}
        >
          {rejectLoading ? 'Rejecting…' : 'Reject'}
        </button>
        <button
          className={styles.approveBtn}
          onClick={submitApprove}
          disabled={busy}
        >
          {approveLoading ? 'Approving…' : 'Approve & advance'}
        </button>
      </div>
    </div>
  );
}
