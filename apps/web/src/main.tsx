import React, {FormEvent, useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import './styles.css';

type Workflow = {id:string; name:string; states:string[]; fields:{key:string;label:string;type:string;required:boolean}[]};
type AuditEvent = {id:string;type:string;recordId:string;occurredAt:string};
const API = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

function App(){
  const [tenant,setTenant]=useState('acme'); const [workflows,setWorkflows]=useState<Workflow[]>([]); const [audit,setAudit]=useState<AuditEvent[]>([]); const [message,setMessage]=useState('');
  const headers={'Content-Type':'application/json','X-Tenant-ID':tenant};
  async function refresh(){
    const [w,a]=await Promise.all([fetch(`${API}/api/v1/workflows`,{headers}),fetch(`${API}/api/v1/audit-events`,{headers})]);
    if(w.ok) setWorkflows(await w.json()); if(a.ok) setAudit(await a.json());
  }
  useEffect(()=>{refresh().catch(()=>setMessage('Start the Go API on :8080 to use the dashboard.'));},[tenant]);
  async function createWorkflow(e:FormEvent<HTMLFormElement>){e.preventDefault(); const f=new FormData(e.currentTarget); const body={name:f.get('name'),states:['SUBMITTED','MANAGER_REVIEW','APPROVED','FULFILLED'],fields:[{key:'request',label:'Request',type:'text',required:true}]}; const r=await fetch(`${API}/api/v1/workflows`,{method:'POST',headers,body:JSON.stringify(body)}); setMessage(r.ok?'Workflow created':'Could not create workflow'); if(r.ok){e.currentTarget.reset(); await refresh();}}
  async function createRecord(wf:Workflow){const request=prompt(`Create a ${wf.name} request`); if(!request)return; const r=await fetch(`${API}/api/v1/records`,{method:'POST',headers:{...headers,'Idempotency-Key':crypto.randomUUID()},body:JSON.stringify({workflowId:wf.id,data:{request}})}); setMessage(r.ok?'Request submitted':'Request failed'); await refresh();}
  return <main><header><div><p className="eyebrow">Enterprise Studio</p><h1>Workflow Platform</h1><p>Build reusable internal apps and tenant-isolated workflows.</p></div><label>Tenant<input value={tenant} onChange={e=>setTenant(e.target.value)}/></label></header>
    {message&&<div className="notice">{message}</div>}
    <section className="grid"><article><h2>Create workflow</h2><form onSubmit={createWorkflow}><input name="name" placeholder="e.g. Equipment Request" required/><button>Create</button></form><p className="muted">New workflows use a four-stage approval lifecycle and a schema-driven request field.</p></article><article><h2>Platform guarantees</h2><ul><li>Tenant-scoped data</li><li>Idempotent creates</li><li>Optimistic concurrency</li><li>Audit trail</li></ul></article></section>
    <section><div className="sectionTitle"><h2>Apps</h2><button className="secondary" onClick={()=>refresh()}>Refresh</button></div><div className="cards">{workflows.map(w=><article className="card" key={w.id}><span className="pill">v1</span><h3>{w.name}</h3><p>{w.states.join(' → ')}</p><button onClick={()=>createRecord(w)}>New request</button></article>)}{!workflows.length&&<p className="muted">No workflows for this tenant yet.</p>}</div></section>
    <section><h2>Recent audit activity</h2><div className="audit">{audit.slice().reverse().map(a=><div key={a.id}><strong>{a.type}</strong><span>{a.recordId}</span><time>{new Date(a.occurredAt).toLocaleString()}</time></div>)}</div></section>
  </main>
}
createRoot(document.getElementById('root')!).render(<React.StrictMode><App/></React.StrictMode>);
