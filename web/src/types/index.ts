// ─── Auth ────────────────────────────────────────────────────────────────────
export interface AuthResponse {
  token: string;
  user: { id: string; username: string };
}

// ─── Workflow ────────────────────────────────────────────────────────────────
export interface Workflow {
  id: string;
  name: string;
  status: string;
  current_phase: number;
  total_phases: number;
  progress?: number;
  project_id?: string;
  created_at?: string;
}

export interface Phase {
  id: string;
  workflow_id: string;
  name: string;
  phase_type: string;
  task_type: string;
  status: string;
  index: number;
  config?: Record<string, unknown>;
}

export interface PhaseWithTasks extends Phase {
  tasks?: Task[];
  total_tasks?: number;
  completed_tasks?: number;
}

export interface Task {
  id: string;
  title: string;
  description?: string;
  result?: string;
  status: string;
  priority?: string;
  type?: string;
  assignee?: string;
  phase_id?: string;
  dependencies?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowTemplate {
  id: string;
  name: string;
  description?: string;
  phases: Phase[];
}

export interface WorkflowListResponse {
  workflows: Workflow[];
}

export interface WorkflowDetailResponse {
  workflow: Workflow;
  phases: PhaseWithTasks[];
  progress: { percentage: number; tasks_done: number; tasks_total: number };
}

export interface StartWorkflowBody {
  name: string;
  template_id?: string;
  project_id?: string;
}

// ─── Project ─────────────────────────────────────────────────────────────────
export interface Project {
  id: string;
  name: string;
  description?: string;
  status: string;
  task_count?: number;
  created_at: string;
}

export interface ProjectDetailResponse {
  project: Project;
  features: Feature[];
  stats: { total_tasks: number; completed: number; completion_rate: number };
}

export interface Feature {
  id: string;
  name: string;
  status: string;
  task_count: number;
  completed_tasks: number;
}

// ─── Agent ───────────────────────────────────────────────────────────────────
export interface Agent {
  id: string;
  name: string;
  role: string;
  status: string;
  model?: string;
  current_tasks: number;
  max_tasks: number;
  total_completed: number;
  total_failed: number;
}

// ─── Dashboard ───────────────────────────────────────────────────────────────
export interface AgentInfo {
  id: string;
  name: string;
  status: string;
  current_tasks: number;
}

export interface RecentTask {
  id: string;
  title: string;
  status: string;
}

export interface DashboardSummary {
  task_counts: { status: string; count: number }[];
  agents: AgentInfo[];
  recent_tasks: RecentTask[];
}
