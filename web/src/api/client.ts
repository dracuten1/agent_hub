import type {
  AuthResponse,
  DashboardSummary,
  Workflow,
  WorkflowListResponse,
  WorkflowDetailResponse,
  WorkflowTemplate,
  StartWorkflowBody,
  Project,
  ProjectListResponse,
  ProjectDetailResponse,
  Agent,
} from '../types';

const BASE = '/api';

function headers(): HeadersInit {
  const h: HeadersInit = { 'Content-Type': 'application/json' };
  const token = localStorage.getItem('token');
  if (token) h['Authorization'] = `Bearer ${token}`;
  return h;
}

async function get<T>(path: string): Promise<T> {
  const r = await fetch(BASE + path, { headers: headers() });
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }));
    throw new Error(err.error ?? 'Request failed');
  }
  return r.json();
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const r = await fetch(BASE + path, {
    method: 'POST',
    headers: headers(),
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }));
    throw new Error(err.error ?? 'Request failed');
  }
  return r.json();
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
export const apiLogin = (username: string, password: string) =>
  post<AuthResponse>('/auth/login', { username, password });

// ─── Dashboard ─────────────────────────────────────────────────────────────────
export const apiDashboardSummary = () => get<DashboardSummary>('/dashboard');

// ─── Workflows ─────────────────────────────────────────────────────────────────
export const apiWorkflows = () => get<WorkflowListResponse>('/workflows');
export const apiWorkflow = (id: string) => get<WorkflowDetailResponse>(`/workflows/${id}`);
export const apiWorkflowTemplates = () => get<{ templates: WorkflowTemplate[] }>('/workflows/templates');
export const apiStartWorkflow = (body: StartWorkflowBody) =>
  post<{ workflow: Workflow }>('/workflows/start', body);
export const apiApproveWorkflow = (id: string, note?: string) =>
  post<{ message: string }>(`/workflows/${id}/approve`, note ? { note } : undefined);
export const apiRejectWorkflow = (id: string, note?: string) =>
  post<{ message: string }>(`/workflows/${id}/reject`, note ? { note } : undefined);

// ─── Projects ──────────────────────────────────────────────────────────────────
export const apiProjects = () => get<ProjectListResponse>('/projects');
export const apiProject = (id: string) => get<ProjectDetailResponse>(`/projects/${id}`);

// ─── Agents ────────────────────────────────────────────────────────────────────
export const apiAgents = () => get<{ agents: Agent[] }>('/agents');

// ─── Tasks ─────────────────────────────────────────────────────────────────────
export const apiTasks = (params?: Record<string, string>) => {
  const qs = params ? '?' + new URLSearchParams(params).toString() : '';
  return get<{ tasks: unknown[] }>(`/tasks${qs}`);
};
