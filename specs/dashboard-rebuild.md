# AgentHub Dashboard Rebuild — React 18 + TypeScript + Vite

> **Status:** Draft  
> **Date:** 2026-04-03  
> **Project:** `/root/.openclaw/workspace-pm/projects/agenthub/`  
> **Build:** `export PATH=$PATH:/usr/local/go/bin && go build ./...`

---

## 1. Overview

Rebuild the AgentHub dashboard frontend with React 18, TypeScript, and Vite. Go backend serves the compiled static files from `web/dist/`. Dark theme only.

**Key changes:**
- Full TypeScript conversion (JSX → TSX)
- New pages: Dashboard, Workflows, Workflow Detail, Projects, Project Detail, Agents
- New components: `WorkflowFlow` (SVG pipeline graph), `PhaseNode`, `TaskList`, `AgentCard`, `ProjectCard`
- Mobile-first: bottom tabs, horizontally scrollable workflow graph, touch targets ≥44px
- Go serves `web/dist/` as static files
- New API endpoints for workflows and projects

---

## 2. Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Frontend framework | React | 18.x |
| Language | TypeScript | 5.x |
| Bundler | Vite | 5.x |
| Routing | React Router DOM | 6.x |
| HTTP client | Fetch (native) | — |
| Icons | Lucide React | — |
| Backend | Go + Gin | 1.21 |
| Database | PostgreSQL 15 + sqlx | — |

**Dependencies to add:**
```json
{
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.0",
    "lucide-react": "^0.447.0"
  },
  "devDependencies": {
    "typescript": "^5.5.0",
    "@types/react": "^18.3.0",
    "@types/react-dom": "^18.3.0",
    "@types/dompurify": "^3.0.0",
    "@vitejs/plugin-react": "^4.3.0",
    "vite": "^5.4.0"
  }
}
```

**Dependencies to remove:** `dompurify` (use DOMPurify types only), `@eslint/*` (defer linting to later), `globals`

---

## 3. Design System

### Color Tokens (CSS Variables)

All colors reference CSS variables. No hardcoded hex values in components.

```css
/* Backgrounds */
--bg:           #0f172a;   /* page background */
--bg-surface:   #1e293b;   /* cards, panels */
--bg-raised:    #334155;   /* hover states, selected items */
--bg-overlay:   #0f172ae6; /* modals, dropdowns (with alpha) */

/* Text */
--text:         #f1f5f9;   /* primary text */
--text-muted:   #94a3b8;   /* secondary text, labels */
--text-faint:   #64748b;   /* placeholder, disabled */

/* Borders */
--border:       #334155;
--border-hover: #475569;

/* Accents */
--primary:      #6366f1;
--primary-hover:#818cf8;
--primary-glow: rgba(99, 102, 241, 0.15);
--success:      #22c55e;
--warning:      #f59e0b;
--danger:       #ef4444;

/* Status */
--status-available:  #22c55e;
--status-claimed:   #f59e0b;
--status-progress:  #6366f1;
--status-review:    #a855f7;
--status-done:     #22c55e;
--status-failed:   #ef4444;
--status-escalated:#f59e0b;

/* Layout */
--radius-sm:  4px;
--radius:     8px;
--radius-lg:  12px;
--radius-xl:  16px;
--navbar-h:   60px;
--bottom-tab-h: 64px;
```

### Typography

- Font: Inter (loaded via Google Fonts or self-hosted)
- Scale: 12 / 14 / 16 / 20 / 24 / 32px
- Weights: 400 (body), 500 (label), 600 (heading), 700 (title)

### Spacing

8px grid: 4 / 8 / 12 / 16 / 24 / 32 / 48px

### Motion

- Transitions: 150ms ease for color/opacity, 200ms ease-out for transforms
- No heavy animations (performance on low-end devices)
- `prefers-reduced-motion` respected

---

## 4. Architecture

### 4.1 File Structure

```
web/src/
  main.tsx                    # entry point
  App.tsx                     # router + layout
  index.css                   # design tokens + global styles
  api/
    client.ts                 # fetch wrapper + auth token
    types.ts                  # shared API response types
  components/
    shared/
      Badge.tsx               # status badge
      Card.tsx                # base card wrapper
      Spinner.tsx             # loading spinner
      ErrorMessage.tsx         # error display
      EmptyState.tsx          # no-data placeholder
      ConfirmDialog.tsx        # delete confirmation modal
    layout/
      Navbar.tsx               # top nav (desktop)
      BottomTabs.tsx           # bottom tabs (mobile)
      PageLayout.tsx           # top nav + main content wrapper
  pages/
    Dashboard.tsx             # stats, pipeline, events
    Workflows.tsx             # workflow list + start form
    WorkflowDetail.tsx        # workflow graph + task list
    Projects.tsx             # project list + create
    ProjectDetail.tsx         # project features + stats
    Agents.tsx                # agent cards + health
    Login.tsx                 # login/register (no nav)
  components/
    WorkflowFlow.tsx          # SVG pipeline graph
    PhaseNode.tsx             # single phase in the graph
    TaskList.tsx              # task list with filters
    AgentCard.tsx             # agent status card
    ProjectCard.tsx           # project summary card
  hooks/
    useFetch.ts               # data fetching with loading/error
    useInterval.ts             # polling for auto-refresh
    useAuth.ts                # auth state from localStorage
  types/
    index.ts                  # all TypeScript interfaces
```

### 4.2 Component Diagram

```
                    App (Router)
                        │
         ┌──────────────┼──────────────┐
    LoginPage      PageLayout          ...
         │              │
    ┌────┴────┐    ┌────┴────┐
  Navbar    BottomTabs
                │
    ┌──────────┼──────────┬──────────┬──────────┐
Dashboard  Workflows  Projects  Agents  (no nav)
    │          │          │          │
WorkflowFlow  WorkflowFlow ProjectCard AgentCard
    │          │
PhaseNode  TaskList
            TaskList → Badge, EmptyState
```

### 4.3 Backend Static File Serving

File: `cmd/server/main.go`

```go
import (
    "embed"
    "io/fs"
    "net/http"
    "path"
)

//go:embed web/dist
var staticFiles embed.FS

// In main():
// Strip "web/dist" prefix from embedded FS
static, _ := fs.Sub(staticFiles, "web/dist")
r.StaticFS("/app", http.FS(static))

// SPA fallback: all non-API routes serve index.html
r.NoRoute(func(c *gin.Context) {
    if !strings.HasPrefix(c.Request.URL.Path, "/api") {
        c.Header("Cache-Control", "no-cache")
        c.FileFromFS("/", http.FS(static))
    } else {
        c.Status(http.StatusNotFound)
    }
})
```

**Note:** Build step must run `vite build` before `go build`. Add a `Makefile` or `justfile` for the build pipeline:

```makefile
.PHONY: build
build:
    cd web && npm install && npm run build && cd ..
    go build -o agenthub ./cmd/server
```

---

## 5. Pages

### 5.1 Dashboard (`/`)

**Purpose:** At-a-glance overview of all agents and task pipeline.

**Sections:**
1. **Header** — "AgentHub" brand, user avatar (top-right), refresh button
2. **Agent Health Strip** — horizontal row of agent cards (health status dot + name + task count)
3. **Task Pipeline** — horizontal Kanban columns: Available | In Progress | Review | Test | Done | Escalated
4. **Recent Events** — scrollable list of task events (last 20)

**Auto-refresh:** 10s polling via `useInterval` hook.

**Mobile:** Agent health strip scrolls horizontally. Pipeline columns stack vertically.

### 5.2 Workflows (`/workflows`)

**Purpose:** List all workflows with status, progress bar, and "Start Workflow" button.

**Layout:**
- Filter bar: status dropdown (all / active / complete / cancelled)
- Workflow cards (ProjectCard style) — name, status badge, phase progress, created date
- FAB: "+ Start Workflow" (bottom-right on mobile, top-right on desktop)

**Start Workflow Dialog:**
- Template name input (with autocomplete from `/api/workflows/templates`)
- Workflow name input
- Project ID input (optional)
- Description textarea
- "Start" button → `POST /api/workflows/start`

### 5.3 Workflow Detail (`/workflows/:id`)

**Purpose:** Visual pipeline graph + task list for one workflow.

**Layout:**
1. **Header** — workflow name, status badge, created date, "Delete" button (admin), "Approve Gate" button (if gate phase active)
2. **WorkflowFlow** — SVG pipeline graph (see §6.1)
3. **Task List** — tasks filtered by selected phase (click phase to filter)

**Interactions:**
- Click a phase node → filter task list to that phase
- Hover phase node → show tooltip: phase name, status, X/Y tasks
- Workflow auto-refreshes every 10s

### 5.4 Projects (`/projects`)

**Purpose:** List all projects.

**Layout:**
- Filter bar: status dropdown, search input
- Project cards (ProjectCard component) — name, description excerpt, feature count, status badge
- FAB: "+ New Project"

**New Project Dialog:**
- Name input
- Description textarea
- "Create" → `POST /api/projects`

### 5.5 Project Detail (`/projects/:id`)

**Purpose:** Single project with features + task stats.

**Layout:**
- Header: project name, description, status badge, "Edit" button
- Stats row: total features, total tasks, completion rate
- Features list (accordion) — click to expand and see tasks within each feature
- Task list filtered by project

### 5.6 Agents (`/agents`)

**Purpose:** Agent health and capacity.

**Layout:**
- Agent cards (AgentCard component) in a 2-column grid
- Each card: agent name, role, status dot, current/max tasks, last heartbeat, health bar
- Stats: X healthy / Y warning / Z dead

**Health logic (frontend):**
- Online: heartbeat < 2 min ago
- Warning: 2–5 min ago
- Dead: > 5 min ago

---

## 6. Components

### 6.1 WorkflowFlow (SVG Pipeline Graph)

**Purpose:** Visual representation of workflow phases as connected nodes.

```
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  Plan   │───▶│  Dev    │───▶│ Review  │───▶│ Deploy  │
│  ✓ 1/1  │    │ ▶ 2/3  │    │ ○ 0/3   │    │ ○ 0/1   │
└─────────┘    └─────────┘    └─────────┘    └─────────┘
```

**Implementation:** Pure SVG, no canvas.

**Layout algorithm:**
1. Compute node positions: `(index * (nodeWidth + gap)) + padding`
2. Draw connecting lines between consecutive nodes (bezier curves)
3. Arrow head at destination end
4. Animate active node with subtle pulse

**Node states:**
| Status | Visual |
|--------|--------|
| `pending` | Dashed border, muted text, grey fill |
| `running` / `active` | Solid border, primary color, pulse animation |
| `waiting_approval` | Amber border, clock icon |
| `completed` | Success border, checkmark icon, muted fill |

**Node width:** 120px. Height: 80px. Gap: 40px.

**Horizontal scroll:** Wrapped in `overflow-x: auto` with `-webkit-overflow-scrolling: touch`.

**Mobile:** Full width scrollable, nodes slightly smaller (100px × 68px).

### 6.2 PhaseNode

**Purpose:** Single phase node inside WorkflowFlow.

**Props:**
```typescript
interface PhaseNodeProps {
  phase: PhaseData;
  x: number;
  y: number;
  isActive: boolean;
  onClick: (phaseId: string) => void;
}

interface PhaseData {
  id: string;
  name: string;
  type: string;         // single, multi, gate, etc.
  status: string;
  totalTasks: number;
  completedTasks: number;
  isCurrentPhase: boolean;
}
```

**Content:**
- Phase name (bold, top)
- Status icon (center): checkmark | spinner | clock | circle
- Task count (bottom): "2/3" or "0/0"

### 6.3 TaskList

**Purpose:** Filterable, sortable task list.

**Props:**
```typescript
interface TaskListProps {
  tasks: Task[];
  filter?: 'all' | PhaseFilter;
  onTaskClick?: (taskId: string) => void;
}
```

**Features:**
- Filter by status: All | Available | In Progress | Review | Done | Failed
- Sort by: Priority | Created | Updated
- Each row: status badge | title (truncated) | assignee | priority dot | updated time
- Click row → expand to show description
- Empty state with icon

**Mobile:** Single column, swipe-to-reveal actions (defer).

### 6.4 AgentCard

**Purpose:** Compact agent status display.

**Props:**
```typescript
interface AgentCardProps {
  agent: Agent;
  onHeartbeat?: (agentId: string) => void;
}
```

**Content:**
- Status dot (green/amber/red) + agent name
- Role badge
- Task progress bar: "2 / 3 tasks"
- Last heartbeat: "5m ago"
- Model name (if set)

### 6.5 ProjectCard

**Purpose:** Compact project summary.

**Props:**
```typescript
interface ProjectCardProps {
  project: Project;
  onClick?: (projectId: string) => void;
}
```

**Content:**
- Project name (bold)
- Status badge
- Description excerpt (2 lines, ellipsis)
- Footer: X features · Y tasks · created date

---

## 7. API Changes

### 7.1 New Endpoints

#### `GET /api/workflows` (already exists — verify)
Response shape:
```typescript
interface WorkflowListResponse {
  workflows: Workflow[];
  total: number;
}
```

#### `GET /api/workflows/:id` (already exists — verify)
Response shape:
```typescript
interface WorkflowDetailResponse {
  workflow: Workflow;
  phases: EnrichedPhase[];
  progress: { total_tasks: number; completed_tasks: number; percentage: number };
}

interface EnrichedPhase {
  id: string;
  workflow_id: string;
  phase_index: number;
  phase_type: string;
  phase_name: string;
  status: string;
  total_tasks: number;
  completed_tasks: number;
  config: Record<string, unknown>;
  tasks?: TaskSummary[];
}

interface TaskSummary {
  id: string;
  title: string;
  status: string;
  assignee: string | null;
}
```

#### `DELETE /api/workflows/:id` (new — see specs/delete-workflow.md)
Requires admin role.

Response 200:
```json
{
  "message": "Workflow deleted",
  "workflow_id": "uuid",
  "deleted_type": "hard",
  "summary": {
    "phases_removed": 8,
    "task_mappings_removed": 12,
    "tasks_released": 3
  }
}
```

#### `GET /api/projects` (already exists — verify)
Response shape:
```typescript
interface ProjectListResponse {
  projects: Project[];
}
```

#### `GET /api/projects/:id` (new or existing)
Response shape:
```typescript
interface ProjectDetailResponse {
  project: Project;
  features: FeatureWithStats[];
  stats: {
    total_features: number;
    total_tasks: number;
    completed_tasks: number;
    completion_rate: number;
  };
}
```

### 7.2 Updated Frontend API Client

File: `web/src/api/client.ts`

```typescript
const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8081';

interface RequestOptions extends RequestInit {
  token?: string;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { token, ...fetchOptions } = options;
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { ...fetchOptions, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error ?? res.statusText);
  return data as T;
}

export const api = {
  auth: {
    login: (username: string, password: string) =>
      request<AuthResponse>('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
    register: (username: string, email: string, password: string) =>
      request<AuthResponse>('/api/auth/register', { method: 'POST', body: JSON.stringify({ username, email, password }) }),
    profile: (token: string) => request<UserProfile>('/api/auth/profile', { token }),
  },
  workflows: {
    list: (token: string) => request<WorkflowListResponse>('/api/workflows', { token }),
    get: (id: string, token: string) => request<WorkflowDetailResponse>(`/api/workflows/${id}`, { token }),
    start: (body: StartWorkflowBody, token: string) =>
      request<WorkflowDetailResponse>('/api/workflows/start', { method: 'POST', body: JSON.stringify(body), token }),
    delete: (id: string, token: string) =>
      request<DeleteResponse>(`/api/workflows/${id}`, { method: 'DELETE', token }),
    approve: (id: string, body: { note: string }, token: string) =>
      request(`/api/workflows/${id}/approve`, { method: 'POST', body: JSON.stringify(body), token }),
    templates: (token: string) => request<{ templates: WorkflowTemplate[] }>('/api/workflows/templates', { token }),
  },
  projects: {
    list: (token: string) => request<ProjectListResponse>('/api/projects', { token }),
    get: (id: string, token: string) => request<ProjectDetailResponse>(`/api/projects/${id}`, { token }),
    create: (body: CreateProjectBody, token: string) =>
      request<{ project: Project }>('/api/projects', { method: 'POST', body: JSON.stringify(body), token }),
    delete: (id: string, token: string) => request(`/api/projects/${id}`, { method: 'DELETE', token }),
  },
  agents: {
    list: (token: string) => request<{ agents: Agent[] }>('/api/agents', { token }),
    health: (token: string) => request<AgentHealthResponse>('/api/agents/health', { token }),
  },
  dashboard: {
    summary: () => request<DashboardSummary>('/api/dashboard/summary'),
  },
};
```

### 7.3 Shared Types

File: `web/src/types/index.ts`

```typescript
// ── Auth ─────────────────────────────────────────────────────────────────────
interface AuthResponse { token: string; user: UserInfo; }
interface UserInfo { id: string; username: string; email: string; role: string; }

// ── Workflows ────────────────────────────────────────────────────────────────
interface Workflow {
  id: string; name: string; status: string;
  current_phase: number; total_phases: number;
  project_id?: string; description?: string;
  created_at: string; updated_at: string;
}
interface Phase {
  id: string; workflow_id: string; phase_index: number;
  phase_type: string; phase_name: string; status: string;
  total_tasks: number; completed_tasks: number;
  config: Record<string, unknown>;
}
interface PhaseWithTasks extends Phase {
  tasks?: TaskSummary[];
}
interface TaskSummary {
  id: string; title: string; status: string; assignee: string | null;
}
interface WorkflowTemplate {
  id: string; name: string; description: string;
  phases: PhaseConfig[];
}
interface PhaseConfig {
  name: string; type: string; task_type: string; config: Record<string, unknown>;
}

// ── Projects ────────────────────────────────────────────────────────────────
interface Project {
  id: string; name: string; description: string;
  status: string; user_id: string;
  feature_count?: number; task_count?: number;
  created_at: string;
}
interface Feature { id: string; name: string; description: string; status: string; }

// ── Agents ──────────────────────────────────────────────────────────────────
interface Agent {
  id: string; name: string; role: string;
  status: string; model?: string;
  current_tasks: number; max_tasks: number;
  total_completed: number; total_failed: number;
  last_heartbeat?: string;
}
type AgentHealth = { healthy: number; warning: number; dead: number; };

// ── Tasks ───────────────────────────────────────────────────────────────────
interface Task {
  id: string; title: string; description: string;
  status: string; priority: string; task_type: string;
  assignee: string | null; progress: number;
  created_at: string; updated_at: string;
}

// ── Dashboard ───────────────────────────────────────────────────────────────
interface DashboardSummary {
  task_counts: { status: string; count: number }[];
  queue: { task_type: string; count: number }[];
  agents: AgentInfo[];
  recent_tasks: RecentTask[];
}
```

---

## 8. Mobile Layout

### 8.1 Bottom Tabs

Always visible on mobile (< 768px). Hidden on desktop.

| Tab | Icon | Label |
|-----|------|-------|
| Dashboard | LayoutDashboard | Home |
| Workflows | GitBranch | Flows |
| Projects | FolderKanban | Projects |
| Agents | Users | Agents |

### 8.2 Navbar (Desktop Only)

Top fixed navbar with:
- Left: "AgentHub" brand + page title
- Right: user avatar + name + logout

### 8.3 Responsive Breakpoints

```css
/* Mobile-first */
.mobile-only { display: flex; }
.desktop-only { display: none; }

@media (min-width: 768px) {
  .mobile-only { display: none; }
  .desktop-only { display: flex; }
}
```

### 8.4 Touch Targets

All interactive elements: minimum 44×44px.

```css
button, a, [role="button"] {
  min-height: 44px;
  min-width: 44px;
}
```

### 8.5 Workflow Graph Mobile

- Horizontal scroll with momentum scrolling
- Node width: 100px (vs 120px desktop)
- Touch: tap node → select phase (visual highlight)
- Active node stays in view when selected (`scrollIntoView({ inline: 'center' })`)

---

## 9. Routing

File: `web/src/App.tsx`

```typescript
import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import PageLayout from './components/layout/PageLayout';
import Dashboard from './pages/Dashboard';
import Workflows from './pages/Workflows';
import WorkflowDetail from './pages/WorkflowDetail';
import Projects from './pages/Projects';
import ProjectDetail from './pages/ProjectDetail';
import Agents from './pages/Agents';
import Login from './pages/Login';

function ProtectedRoute({ children }: { children: JSX.Element }) {
  const { token } = useAuth();
  if (!token) return <Navigate to="/login" replace />;
  return children;
}

const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    path: '/',
    element: <ProtectedRoute><PageLayout /></ProtectedRoute>,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'workflows', element: <Workflows /> },
      { path: 'workflows/:id', element: <WorkflowDetail /> },
      { path: 'projects', element: <Projects /> },
      { path: 'projects/:id', element: <ProjectDetail /> },
      { path: 'agents', element: <Agents /> },
    ],
  },
]);
```

---

## 10. Build Pipeline

### Go Backend

The backend reads from `web/dist/`. During development, Vite dev server proxies `/api` to the Go backend.

```makefile
# Makefile
.PHONY: build dev clean

build: web-build
	go build -o bin/server ./cmd/server

web-build:
	cd web && npm ci && npm run build

dev:
	cd web && npm run dev &
	go run ./cmd/server

clean:
	rm -rf bin/ web/dist
```

### Vite Config Update

File: `web/vite.config.ts`

```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: undefined,  // single bundle for simplicity
      },
    },
  },
});
```

---

## 11. Implementation Tasks

### Wave 1: TypeScript Setup + Routing (dev1, 60 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Install TypeScript + types + react-router | `package.json` |
| 2 | Add `tsconfig.json` | `web/tsconfig.json` |
| 3 | Add `vite.config.ts` | `web/vite.config.ts` |
| 4 | Rename `main.jsx` → `main.tsx` | `web/src/main.tsx` |
| 5 | Create `types/index.ts` | `web/src/types/index.ts` |
| 6 | Create `api/client.ts` | `web/src/api/client.ts` |
| 7 | Create `hooks/useAuth.ts` | `web/src/hooks/useAuth.ts` |
| 8 | Create `hooks/useFetch.ts` | `web/src/hooks/useFetch.ts` |
| 9 | Create `hooks/useInterval.ts` | `web/src/hooks/useInterval.ts` |
| 10 | Convert `App.jsx` → `App.tsx` with routing | `web/src/App.tsx` |
| 11 | Create `PageLayout.tsx` | `web/src/components/layout/PageLayout.tsx` |
| 12 | Create `Navbar.tsx` + `BottomTabs.tsx` | `web/src/components/layout/` |
| 13 | Update `index.css` with design tokens + responsive | `web/src/index.css` |
| 14 | Convert `Login.jsx` → `Login.tsx` | `web/src/pages/Login.tsx` |

### Wave 2: Backend Additions (dev2, 45 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Add `DELETE /api/workflows/:id` handler | `internal/workflow/handler.go` |
| 2 | Register `DELETE /:id` route in `RegisterRoutes` | `internal/workflow/handler.go` |
| 3 | Add `GET /api/projects/:id` with features + stats | `internal/project/handler.go` |
| 4 | Wire Go static file serving from `web/dist/` | `cmd/server/main.go` |
| 5 | Add SPA fallback `NoRoute` handler | `cmd/server/main.go` |

### Wave 3: Pages (dev1, 90 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Convert `Dashboard.jsx` → `Dashboard.tsx` | `web/src/pages/Dashboard.tsx` |
| 2 | Create `Workflows.tsx` | `web/src/pages/Workflows.tsx` |
| 3 | Create `WorkflowDetail.tsx` | `web/src/pages/WorkflowDetail.tsx` |
| 4 | Create `Projects.tsx` | `web/src/pages/Projects.tsx` |
| 5 | Create `ProjectDetail.tsx` | `web/src/pages/ProjectDetail.tsx` |
| 6 | Create `Agents.tsx` | `web/src/pages/Agents.tsx` |

### Wave 4: Components (dev1, 90 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Create `WorkflowFlow.tsx` (SVG graph) | `web/src/components/WorkflowFlow.tsx` |
| 2 | Create `PhaseNode.tsx` | `web/src/components/PhaseNode.tsx` |
| 3 | Create `TaskList.tsx` | `web/src/components/TaskList.tsx` |
| 4 | Create `AgentCard.tsx` | `web/src/components/AgentCard.tsx` |
| 5 | Create `ProjectCard.tsx` | `web/src/components/ProjectCard.tsx` |
| 6 | Create shared: `Badge`, `Card`, `Spinner`, `EmptyState`, `ConfirmDialog` | `web/src/components/shared/` |

### Wave 5: Mobile Polish (dev1, 30 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Bottom tabs show/hide by breakpoint | `BottomTabs.tsx` |
| 2 | Workflow graph horizontal scroll + touch | `WorkflowFlow.tsx` |
| 3 | Touch target minimums | `index.css` |
| 4 | Test on 375px width (iPhone SE) | — |

### Wave 6: Integration + Build (both, 30 min)

| # | Subtask | File |
|---|---------|------|
| 1 | Fix any TS compile errors | `*.tsx`, `*.ts` |
| 2 | Run `npm run build` in `web/` | — |
| 3 | Run `go build ./...` | — |
| 4 | Verify SPA routing works with Go fallback | — |

**Total wall time:** ~5.5 hours with 2 devs

---

## 12. Files Summary

### Files to Create
| File | Wave |
|------|------|
| `web/tsconfig.json` | 1 |
| `web/vite.config.ts` | 1 |
| `web/src/types/index.ts` | 1 |
| `web/src/api/client.ts` | 1 |
| `web/src/hooks/useAuth.ts` | 1 |
| `web/src/hooks/useFetch.ts` | 1 |
| `web/src/hooks/useInterval.ts` | 1 |
| `web/src/components/layout/PageLayout.tsx` | 1 |
| `web/src/components/layout/Navbar.tsx` | 1 |
| `web/src/components/layout/BottomTabs.tsx` | 1 |
| `web/src/pages/Login.tsx` | 1 |
| `web/src/pages/Dashboard.tsx` | 3 |
| `web/src/pages/Workflows.tsx` | 3 |
| `web/src/pages/WorkflowDetail.tsx` | 3 |
| `web/src/pages/Projects.tsx` | 3 |
| `web/src/pages/ProjectDetail.tsx` | 3 |
| `web/src/pages/Agents.tsx` | 3 |
| `web/src/components/WorkflowFlow.tsx` | 4 |
| `web/src/components/PhaseNode.tsx` | 4 |
| `web/src/components/TaskList.tsx` | 4 |
| `web/src/components/AgentCard.tsx` | 4 |
| `web/src/components/ProjectCard.tsx` | 4 |
| `web/src/components/shared/Badge.tsx` | 4 |
| `web/src/components/shared/Card.tsx` | 4 |
| `web/src/components/shared/Spinner.tsx` | 4 |
| `web/src/components/shared/EmptyState.tsx` | 4 |
| `web/src/components/shared/ConfirmDialog.tsx` | 4 |
| `Makefile` | 6 |

### Files to Modify
| File | Change |
|------|--------|
| `web/package.json` | Add TypeScript + react-router + lucide-react deps |
| `web/src/App.jsx` → `App.tsx` | Replace with router setup |
| `web/src/index.css` | Add design tokens, responsive utilities, scrollbar styling |
| `web/src/main.jsx` → `main.tsx` | Import `./index.css` |
| `internal/workflow/handler.go` | Add DELETE handler + route registration |
| `internal/project/handler.go` | Add GET :id with features + stats |
| `cmd/server/main.go` | Add static file serving, SPA fallback, NoRoute |

### Files to Delete
| File | Reason |
|------|--------|
| `web/src/App.css` | Replaced by `index.css` tokens |
| `web/src/components/Dashboard.jsx` | Replaced by `Dashboard.tsx` |
| `web/src/components/Board.jsx` | Replaced by `Projects.tsx` + `Workflows.tsx` |
| `web/src/components/Login.jsx` | Replaced by `Login.tsx` |
| `web/src/api/client.js` | Replaced by `api/client.ts` |

---

## 13. Acceptance Criteria

- [ ] `npm run build` in `web/` produces `web/dist/` with `index.html`
- [ ] `go build ./...` passes with static file embedding
- [ ] `GET /api/*` routes still work (no regression)
- [ ] `GET /app/` serves the React app (SPA)
- [ ] `GET /app/workflows/abc123` serves `index.html` (SPA routing)
- [ ] All 6 pages render with correct layout
- [ ] WorkflowFlow renders pipeline graph for a multi-phase workflow
- [ ] Bottom tabs visible on mobile (≤768px), hidden on desktop
- [ ] Workflow graph scrolls horizontally on mobile
- [ ] `DELETE /api/workflows/:id` works with admin JWT
- [ ] `GET /api/projects/:id` returns features + stats
- [ ] No TypeScript errors (`npx tsc --noEmit` passes)
- [ ] Dark theme on all pages (no white flash on load)

---

*Last updated: 2026-04-03*
