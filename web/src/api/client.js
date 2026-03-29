const API_BASE = 'http://localhost:8081';

let token = localStorage.getItem('auth_token');

export function getToken() {
  return token;
}

export function setToken(t) {
  token = t;
  if (t) localStorage.setItem('auth_token', t);
  else localStorage.removeItem('auth_token');
}

async function request(path, options = {}) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

export const auth = {
  register: (username, email, password) =>
    request('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, email, password }),
    }),
  login: (username, password) =>
    request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
};

export const dashboard = {
  get: () => request('/api/dashboard'),
};

export const agents = {
  health: () => request('/api/agents/health'),
  list: () => request('/api/agents'),
};

export const tasks = {
  list: (status) => request(`/api/tasks${status ? `?status=${status}` : ''}`),
};

export const projects = {
  list: () => request('/api/projects'),
  get: (id) => request(`/api/projects/${id}`),
};

export const features = {
  list: (projectId) => request(`/api/features?project_id=${projectId}`),
};
