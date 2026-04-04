# Spec: Workflow Detail Page Improvement

## Problem
Clicking a workflow phase shows minimal info — no specs, no task results, no context for gates. Users can't make informed approval decisions.

## Current State
- `WorkflowDetail.tsx` — horizontal pipeline with `WorkflowFlow` → `PhaseNode` components
- `PhaseNode` shows: phase name, status badge, task_type, done/total, progress bar, task list (title + assignee + status)
- Gate phases show "paused waiting for approval" with an approve button
- No task result/artifact display
- No phase description
- No expand/collapse interaction

## Component Architecture

### 1. PhaseNode — Make expandable (modify existing)
**File:** `web/src/components/PhaseNode.tsx`

Current: Always shows collapsed info.
New: Click header → toggles expand/collapse.

**Expanded state shows:**
- Phase description (from phase `config.description` or generated summary)
- Full task list with expandable rows
- Each task row click → shows task description + result (artifact)

**Props:** No change — same `PhaseNode` props.

### 2. TaskArtifact — New component
**File:** `web/src/components/TaskArtifact.tsx`

Renders a task's `description` and `result` as formatted content:
- If result is markdown → render as markdown (use simple regex, no library)
- If result is JSON → pretty-print in a code block
- If result is plain text → render as-is in a scrollable pre block
- Show "No result yet" for pending/in_progress tasks

**Props:**
```ts
interface TaskArtifactProps {
  task: Task;
}
```

### 3. PhaseNode.module.css — Add expand styles
**File:** `web/src/components/PhaseNode.module.css`

- `.expandable` cursor pointer on header
- `.expanded` max-height transition for smooth expand
- `.artifact` styled pre/code block for task results
- `.gateSummary` styled box for gate phase context

### 4. WorkflowDetail — Improve gate approval UX
**File:** `web/src/pages/WorkflowDetail.tsx`

Current: Generic "paused waiting for approval" message.
New: Show gate-specific context:
- Summary line: "X/Y tasks completed in previous phase"
- List of previous phase results (collapsed by default)
- Approve button + Reject button (new)
- Note input field for approval/rejection reason

### 5. API — Add reject endpoint to client
**File:** `web/src/api/client.ts`

Add:
```ts
export const apiRejectWorkflow = (id: string, note?: string) =>
  post<{ message: string }>(`/workflows/${id}/reject`, { note });
```

Note: Backend reject endpoint may not exist yet. If 404, show error toast. This is a frontend-only task.

## Interaction Flow

1. User lands on workflow detail → sees horizontal pipeline
2. Clicks any phase → phase card expands showing description + tasks
3. Clicks a task → task expands showing description + result/artifact
4. Click again → collapses
5. At gate phase → sees summary of previous work + approve/reject buttons
6. Clicks approve → calls API → workflow advances
7. Clicks reject → calls API → workflow fails/stops (if backend supports it)

## Atomic Tasks

### Task 1: Make PhaseNode expandable + add task result display
- **Files:** `PhaseNode.tsx`, `PhaseNode.module.css`
- **Scope:** Add click handler on header, expand/collapse animation, render task description + result
- **Create:** `TaskArtifact.tsx` component for rendering results

### Task 2: Improve gate approval UX
- **Files:** `WorkflowDetail.tsx`, `WorkflowDetail.module.css`
- **Scope:** Replace generic approval bar with gate-specific context, add reject button, add note input
- **API:** Add `apiRejectWorkflow` to `client.ts`

## Mock Data
```
Phase 2 (Design):
  config.description: "Team leader writes the design spec for the feature"
  tasks: [
    { title: "Design spec for React Dashboard", status: "done", result: "## Overview\n\nThis spec describes..." },
  ]

Phase 7 (Quality Gate):
  phase_type: "decision"
  config: { pass_condition: "all_tasks_done", auto: false }
  tasks: [] (no tasks — evaluates condition)
```
