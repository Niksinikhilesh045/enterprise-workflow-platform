import React, { FormEvent, useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

type Workflow = {
  id: string;
  name: string;
  states: string[];
  fields: { key: string; label: string; type: string; required: boolean }[];
};
type AuditEvent = { id: string; type: string; recordId: string; occurredAt: string };
type TokenClaims = { sub?: string; tenantId?: string; role?: string; exp?: number };

const API = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

function decodeClaims(token: string): TokenClaims | null {
  try {
    const [payload] = token.split('.');
    if (!payload) return null;
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
    return JSON.parse(atob(padded)) as TokenClaims;
  } catch {
    return null;
  }
}

function App() {
  const [token, setToken] = useState(() => localStorage.getItem('ewp_token') ?? '');
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [message, setMessage] = useState('');
  const claims = useMemo(() => decodeClaims(token), [token]);

  const headers = useMemo(
    () => ({
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    }),
    [token],
  );

  function saveToken(value: string) {
    const trimmed = value.trim();
    setToken(trimmed);
    if (trimmed) localStorage.setItem('ewp_token', trimmed);
    else localStorage.removeItem('ewp_token');
  }

  async function apiFetch(path: string, init: RequestInit = {}) {
    const response = await fetch(`${API}${path}`, {
      ...init,
      headers: { ...headers, ...(init.headers ?? {}) },
    });
    if (response.status === 401) setMessage('Authentication failed. Generate a fresh bearer token.');
    if (response.status === 403) setMessage('Your role is not authorized for that operation.');
    return response;
  }

  async function refresh() {
    if (!token) {
      setWorkflows([]);
      setAudit([]);
      return;
    }
    const workflowsResponse = await apiFetch('/api/v1/workflows');
    if (workflowsResponse.ok) setWorkflows(await workflowsResponse.json());

    // Audit history is intentionally role-protected. A non-auditor receives 403.
    const auditResponse = await apiFetch('/api/v1/audit-events');
    if (auditResponse.ok) setAudit(await auditResponse.json());
    else if (auditResponse.status === 403) setAudit([]);
  }

  useEffect(() => {
    refresh().catch(() => setMessage('Start the Go API on :8080 to use the dashboard.'));
  }, [token]);

  async function createWorkflow(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const body = {
      name: form.get('name'),
      states: ['SUBMITTED', 'MANAGER_REVIEW', 'APPROVED', 'FULFILLED'],
      fields: [{ key: 'request', label: 'Request', type: 'text', required: true }],
    };
    const response = await apiFetch('/api/v1/workflows', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    if (response.ok) {
      setMessage('Workflow created');
      event.currentTarget.reset();
      await refresh();
    }
  }

  async function createRecord(workflow: Workflow) {
    const request = prompt(`Create a ${workflow.name} request`);
    if (!request) return;
    const response = await apiFetch('/api/v1/records', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: JSON.stringify({ workflowId: workflow.id, data: { request } }),
    });
    if (response.ok) {
      setMessage('Request submitted');
      await refresh();
    }
  }

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">Enterprise Studio</p>
          <h1>Workflow Platform</h1>
          <p>Build reusable internal apps with signed, tenant-bound access.</p>
        </div>
      </header>

      <section className="authPanel">
        <div>
          <h2>Bearer access</h2>
          <p className="muted">
            Generate a local token with the Go token CLI, then paste it here. The server verifies its signature, expiry,
            tenant, and role.
          </p>
        </div>
        <textarea
          aria-label="Bearer token"
          value={token}
          onChange={(event) => saveToken(event.target.value)}
          placeholder="Paste bearer token"
          rows={3}
        />
        <div className="claims">
          <span>Tenant: <strong>{claims?.tenantId ?? '—'}</strong></span>
          <span>Role: <strong>{claims?.role ?? '—'}</strong></span>
          <span>User: <strong>{claims?.sub ?? '—'}</strong></span>
        </div>
      </section>

      {message && <div className="notice">{message}</div>}

      <section className="grid">
        <article>
          <h2>Create workflow</h2>
          <form onSubmit={createWorkflow}>
            <input name="name" placeholder="e.g. Equipment Request" required />
            <button disabled={!token}>Create</button>
          </form>
          <p className="muted">Requires the admin or builder role.</p>
        </article>
        <article>
          <h2>Platform guarantees</h2>
          <ul>
            <li>Signed tenant-bound identity</li>
            <li>Role-based authorization</li>
            <li>Idempotent creates</li>
            <li>Optimistic concurrency</li>
            <li>Transactional audit/outbox writes</li>
          </ul>
        </article>
      </section>

      <section>
        <div className="sectionTitle">
          <h2>Apps</h2>
          <button className="secondary" onClick={() => refresh()} disabled={!token}>Refresh</button>
        </div>
        <div className="cards">
          {workflows.map((workflow) => (
            <article className="card" key={workflow.id}>
              <span className="pill">v1</span>
              <h3>{workflow.name}</h3>
              <p>{workflow.states.join(' → ')}</p>
              <button onClick={() => createRecord(workflow)}>New request</button>
            </article>
          ))}
          {!workflows.length && <p className="muted">No accessible workflows yet.</p>}
        </div>
      </section>

      <section>
        <h2>Recent audit activity</h2>
        <p className="muted">Audit history is available only to admin and auditor tokens.</p>
        <div className="audit">
          {audit.slice().reverse().map((event) => (
            <div key={event.id}>
              <strong>{event.type}</strong>
              <span>{event.recordId}</span>
              <time>{new Date(event.occurredAt).toLocaleString()}</time>
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
