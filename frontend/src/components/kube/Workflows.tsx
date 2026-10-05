import { Fragment, useEffect, useMemo, useState } from 'react'
import * as API from '../../../wailsjs/go/main/App'
import { filterK, getK, useKData, type KObj } from '../../kdata'
import { ago } from '../Status'
import { ConfirmButton, duration, KPage, PhaseIcon, PodsAndLogs, Problem1, useAction, VTable, type Column } from './Common'

type Notify = (m: string, ok: boolean) => void
type Sub = 'workflows' | 'templates' | 'cron'

// ---- list views ---------------------------------------------------------------------------

export function WorkflowsView({ query, ctxFilter, onOpen }: { query: string; ctxFilter: Set<string>; onOpen: (key: string) => void }) {
  const data = useKData()
  const [sub, setSub] = useState<Sub>('workflows')
  const [phase, setPhase] = useState<string>('')
  const all = useMemo(() => [...data.objs.values()].filter((o) => !ctxFilter.size || ctxFilter.has(o.ctx)), [data.version, ctxFilter]) // eslint-disable-line react-hooks/exhaustive-deps
  const wfs = all.filter((o) => o.kind === 'Workflow')
  const tmpls = all.filter((o) => o.kind === 'WorkflowTemplate')
  const crons = all.filter((o) => o.kind === 'CronWorkflow')
  const counts: Record<string, number> = {}
  for (const w of wfs) counts[w.phase] = (counts[w.phase] ?? 0) + 1

  const list = useMemo(() => {
    const src = sub === 'workflows' ? wfs.filter((w) => !phase || w.phase === phase) : sub === 'templates' ? tmpls : crons
    const out = filterK(src, query)
    return sub === 'workflows'
      ? out.sort((a, b) => String(b.fields.startedAt || b.created).localeCompare(String(a.fields.startedAt || a.created)))
      : out.sort((a, b) => b.severity - a.severity || a.name.localeCompare(b.name))
  }, [sub, phase, query, data.version, ctxFilter]) // eslint-disable-line react-hooks/exhaustive-deps

  const wfCols: Column<KObj>[] = [
    { header: '', width: '26px', render: (o) => <PhaseIcon kind="Workflow" phase={o.phase} /> },
    { header: 'Workflow', width: 'minmax(220px,2fr)', render: (o) => <b>{o.name}</b> },
    { header: 'Template / cron', width: 'minmax(140px,1.2fr)', render: (o) => <span className="muted">{String(o.fields.cron || o.fields.template || '—')}</span> },
    { header: 'Namespace', width: 'minmax(100px,.8fr)', render: (o) => <span className="muted">{o.namespace}<span className="sub">{o.ctx}</span></span> },
    { header: 'Progress', width: '80px', render: (o) => <span className="mono">{String(o.fields.progress ?? '')}</span> },
    { header: 'Problem', width: 'minmax(220px,3fr)', render: (o) => <Problem1 o={o} /> },
    { header: 'Started', width: '90px', render: (o) => <span className="muted">{ago(String(o.fields.startedAt || o.created))}</span> },
    { header: 'Duration', width: '80px', render: (o) => <span className="muted">{duration(String(o.fields.startedAt ?? ''), String(o.fields.finishedAt || '') || undefined)}</span> },
  ]
  const tmplCols: Column<KObj>[] = [
    { header: 'Template', width: 'minmax(220px,2fr)', render: (o) => <b>{o.name}</b> },
    { header: 'Namespace', width: 'minmax(100px,1fr)', render: (o) => <span className="muted">{o.namespace}<span className="sub">{o.ctx}</span></span> },
    { header: 'Entrypoint', width: 'minmax(100px,1fr)', render: (o) => <span className="mono">{String(o.fields.entrypoint ?? '')}</span> },
    { header: 'Parameters', width: 'minmax(200px,2fr)', render: (o) => <span className="muted">{((o.fields.params as { name: string }[]) ?? []).map((p) => p.name).join(', ')}</span> },
    { header: 'Runs', width: '70px', render: (o) => <span>{wfs.filter((w) => w.fields.template === o.name && w.ctx === o.ctx).length}</span> },
  ]
  const cronCols: Column<KObj>[] = [
    { header: '', width: '26px', render: (o) => <PhaseIcon kind="CronWorkflow" phase={o.severity === 2 ? 'Failed' : o.phase} /> },
    { header: 'CronWorkflow', width: 'minmax(200px,2fr)', render: (o) => <b>{o.name}</b> },
    { header: 'Schedule', width: 'minmax(140px,1.2fr)', render: (o) => <span className="mono">{String(o.fields.schedule ?? '')}<span className="sub">{String(o.fields.timezone ?? '')}</span></span> },
    { header: 'Namespace', width: 'minmax(100px,.8fr)', render: (o) => <span className="muted">{o.namespace}<span className="sub">{o.ctx}</span></span> },
    { header: 'Last run', width: '90px', render: (o) => <span className="muted">{o.fields.lastScheduled ? ago(String(o.fields.lastScheduled)) : '—'}</span> },
    { header: 'Ok / failed', width: '90px', render: (o) => <span>{String(o.fields.succeeded ?? 0)} / <span style={{ color: Number(o.fields.failed) ? 'var(--error-fg)' : undefined }}>{String(o.fields.failed ?? 0)}</span></span> },
    { header: 'Problem', width: 'minmax(200px,2.5fr)', render: (o) => (o.phase === 'Suspended' && !o.problems?.length ? <span className="muted">suspended</span> : <Problem1 o={o} />) },
  ]
  const failing = wfs.filter((w) => w.severity === 2).length
  return (
    <>
      <div className="filterbar">
        <div className="seg small">
          <button className={sub === 'workflows' ? 'on' : ''} onClick={() => setSub('workflows')}>Workflows ({wfs.length})</button>
          <button className={sub === 'templates' ? 'on' : ''} onClick={() => setSub('templates')}>Templates ({tmpls.length})</button>
          <button className={sub === 'cron' ? 'on' : ''} onClick={() => setSub('cron')}>Cron ({crons.length})</button>
        </div>
        {sub === 'workflows' && (
          <>
            <span className="sep" />
            {['Running', 'Pending', 'Succeeded', 'Failed', 'Error'].map((p) => (
              <span key={p} className={'chip' + (phase === p ? ' on' : '')} onClick={() => setPhase(phase === p ? '' : p)}>
                <PhaseIcon kind="Workflow" phase={p} />{p}<span className="n">{counts[p] ?? 0}</span>
              </span>
            ))}
          </>
        )}
        <span className="spacer" />
        {failing > 0 && sub === 'workflows' && <span className="badge error">{failing} failed</span>}
      </div>
      <VTable
        rows={list}
        columns={sub === 'workflows' ? wfCols : sub === 'templates' ? tmplCols : cronCols}
        rowKey={(o) => o.key}
        onOpen={(o) => onOpen(o.key)}
        rowClass={(o) => (o.severity === 2 ? 'sev-2' : '')}
        empty={<NoKube what="workflows" />}
      />
    </>
  )
}

export function NoKube({ what }: { what: string }) {
  const data = useKData()
  if (!data.statuses.length) {
    return (
      <div className="empty">
        <h3>No Kubernetes clusters enabled</h3>
        Argo Workflows, Events and Rollouts are read straight from your clusters. Enable the kubeconfig contexts you want in
        <b> Settings → Kubernetes clusters</b>.
      </div>
    )
  }
  return <div className="empty">No {what} found in the enabled clusters.</div>
}

// ---- workflow page ------------------------------------------------------------------------

type WNode = {
  id: string; displayName: string; type: string; phase: string; message?: string; templateName?: string
  children?: string[]; startedAt?: string; finishedAt?: string
  inputs?: { parameters?: { name: string; value?: string }[]; artifacts?: { name: string }[] }
  outputs?: { parameters?: { name: string; value?: string }[]; artifacts?: { name: string }[]; exitCode?: string }
}

const NW = 210, NH = 46, CG = 60, RG = 12

function layout(nodes: Record<string, WNode>, wfName: string) {
  const ids = Object.keys(nodes)
  const hasParent = new Set<string>()
  for (const id of ids) for (const c of nodes[id].children ?? []) hasParent.add(c)
  const roots = ids.filter((id) => !hasParent.has(id))
  roots.sort((a, b) => (a === wfName ? -1 : b === wfName ? 1 : 0))
  // longest-path layering for DAGs
  const depth: Record<string, number> = {}
  const order: string[] = []
  const visit = (id: string, d: number, guard: Set<string>) => {
    if (guard.has(id)) return
    if (depth[id] === undefined) order.push(id)
    if ((depth[id] ?? -1) >= d) return
    depth[id] = d
    guard.add(id)
    for (const c of nodes[id]?.children ?? []) if (nodes[c]) visit(c, d + 1, guard)
    guard.delete(id)
  }
  for (const r of roots) visit(r, 0, new Set())
  const cols: string[][] = []
  for (const id of order) (cols[depth[id]] ??= []).push(id)
  const pos: Record<string, { x: number; y: number }> = {}
  const maxRows = Math.max(1, ...cols.map((c) => c.length))
  const H = maxRows * (NH + RG)
  cols.forEach((col, d) => {
    const off = (H - col.length * (NH + RG)) / 2
    col.forEach((id, i) => (pos[id] = { x: d * (NW + CG), y: off + i * (NH + RG) }))
  })
  const edges: { a: string; b: string }[] = []
  for (const id of ids) for (const c of nodes[id].children ?? []) if (pos[id] && pos[c]) edges.push({ a: id, b: c })
  return { pos, edges, width: cols.length * (NW + CG), height: H + 20 }
}

const virtualTypes = new Set(['StepGroup', 'TaskGroup'])

export function WorkflowPage({ objKey, left, onClose, notify, onOpenKey }: { objKey: string; left: number; onClose: () => void; notify: Notify; onOpenKey: (k: string) => void }) {
  const data = useKData()
  const o = getK(objKey)
  const [raw, setRaw] = useState<Record<string, any> | null>(null) // eslint-disable-line @typescript-eslint/no-explicit-any
  const [sel, setSel] = useState<string | null>(null)
  const run = useAction(notify)
  const stamp = o ? `${o.phase}|${o.fields.progress}|${o.fields.finishedAt}` : ''
  useEffect(() => {
    API.KObject(objKey).then(setRaw).catch(() => setRaw(null))
  }, [objKey, stamp])
  void data.version
  if (!o) return null
  const nodes: Record<string, WNode> = raw?.status?.nodes ?? {}
  const g = layout(nodes, o.name)
  const node = sel ? nodes[sel] : undefined
  const running = o.phase === 'Running' || o.phase === 'Pending'

  const resubmit = async () => {
    try {
      const k = await API.WorkflowAction(objKey, 'resubmit')
      notify('Resubmitted', true)
      onOpenKey(k)
    } catch (e) {
      notify(`Resubmit failed: ${e}`, false)
    }
  }

  return (
    <KPage
      obj={o}
      left={left}
      onClose={onClose}
      icon="wf"
      subtitle={<>{o.fields.template ? <>template <b>{String(o.fields.template)}</b> · </> : null}progress {String(o.fields.progress ?? '')} · {duration(String(o.fields.startedAt ?? ''), String(o.fields.finishedAt || '') || undefined)}</>}
      actions={
        <>
          {running && !o.fields.suspended && <button className="btn" onClick={() => run('Suspend', () => API.WorkflowAction(objKey, 'suspend'))}>❚❚ Suspend</button>}
          {o.fields.suspended ? <button className="btn primary" onClick={() => run('Resume', () => API.WorkflowAction(objKey, 'resume'))}>▶ Resume</button> : null}
          {running && <ConfirmButton label="■ Stop" confirm="Stop (run exit handlers)?" onConfirm={() => run('Stop', () => API.WorkflowAction(objKey, 'stop'))} />}
          {running && <ConfirmButton label="✕ Terminate" confirm="Terminate now?" onConfirm={() => run('Terminate', () => API.WorkflowAction(objKey, 'terminate'))} />}
          <button className="btn" onClick={resubmit}>↻ Resubmit</button>
          <span className="spacer" />
          <ConfirmButton className="danger-outline" label="🗑 Delete" confirm={`Delete ${o.name}?`} onConfirm={() => run('Delete', () => API.KDelete(objKey)).then(onClose)} />
        </>
      }
      tabs={[
        {
          id: 'graph', label: 'Graph', flush: true,
          render: () => (
            <div className="rtree-main">
              <div className="rtree-canvas" onClick={() => setSel(null)}>
                {!raw && <div className="help" style={{ padding: 16 }}>Loading…</div>}
                <div style={{ width: g.width, height: g.height, position: 'relative', margin: 20 }}>
                  <svg width={g.width} height={g.height} className="rtree-edges">
                    {g.edges.map(({ a, b }) => {
                      const p = g.pos[a], q = g.pos[b]
                      const x1 = p.x + NW, y1 = p.y + NH / 2, x2 = q.x, y2 = q.y + NH / 2, mx = (x1 + x2) / 2
                      const bad = ['Failed', 'Error'].includes(nodes[b]?.phase)
                      return <path key={a + b} d={`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} className={bad ? 'bad' : ''} />
                    })}
                  </svg>
                  {Object.entries(g.pos).map(([id, p]) => {
                    const n = nodes[id]
                    if (virtualTypes.has(n.type)) {
                      return <div key={id} className="wf-dot" style={{ left: p.x + NW / 2 - 7, top: p.y + NH / 2 - 7 }} title={n.displayName} />
                    }
                    const bad = n.phase === 'Failed' || n.phase === 'Error'
                    return (
                      <div key={id} className={'rnode wf-node' + (bad ? ' degraded' : '') + (sel === id ? ' selected' : '') + (n.phase === 'Omitted' || n.phase === 'Skipped' ? ' child' : '')}
                        style={{ left: p.x, top: p.y, width: NW, height: NH, marginLeft: 0 }}
                        onClick={(e) => { e.stopPropagation(); setSel(id) }} title={n.message}>
                        <PhaseIcon kind="Workflow" phase={n.phase} />
                        <div className="rnode-text">
                          <div className="rnode-name">{n.displayName}</div>
                          <div className={'rnode-kind' + (bad ? ' err' : '')}>{bad && n.message ? n.message : `${n.type}${n.templateName ? ' · ' + n.templateName : ''} · ${duration(n.startedAt, n.finishedAt)}`}</div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
              {node && <NodePanel objKey={objKey} node={node} onClose={() => setSel(null)} />}
            </div>
          ),
        },
        { id: 'logs', label: 'Pods & logs', flush: true, render: () => <PodsAndLogs objKey={objKey} /> },
        { id: 'params', label: 'Parameters', render: () => <WfParams raw={raw} /> },
      ]}
    />
  )
}

function NodePanel({ objKey, node, onClose }: { objKey: string; node: WNode; onClose: () => void }) {
  const [tab, setTab] = useState<'summary' | 'logs'>(node.type === 'Pod' ? 'logs' : 'summary')
  useEffect(() => setTab(node.type === 'Pod' ? 'logs' : 'summary'), [node.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const kv = (list?: { name: string; value?: string }[]) =>
    (list ?? []).map((p) => <Fragment key={p.name}><div className="k">{p.name}</div><div className="v mono">{p.value ?? ''}</div></Fragment>)
  return (
    <div className={'rtree-side selectable' + (tab === 'logs' ? ' logs-mode' : '')} style={{ width: tab === 'logs' ? 720 : 380 }}>
      <div className="rtree-side-head">
        <PhaseIcon kind="Workflow" phase={node.phase} />
        <div style={{ minWidth: 0, flex: 1 }}>
          <div className="rnode-name">{node.displayName}</div>
          <div className="rnode-kind">{node.type}{node.templateName ? ` · template ${node.templateName}` : ''}</div>
        </div>
        <button className="x" onClick={onClose}>✕</button>
      </div>
      <div className="seg small" style={{ marginBottom: 12 }}>
        <button className={tab === 'summary' ? 'on' : ''} onClick={() => setTab('summary')}>Summary</button>
        {node.type === 'Pod' && <button className={tab === 'logs' ? 'on' : ''} onClick={() => setTab('logs')}>Logs</button>}
      </div>
      {tab === 'summary' && (
        <>
          {node.message && <div className={'alert' + (['Failed', 'Error'].includes(node.phase) ? '' : ' ok')} style={{ marginBottom: 10 }}>{node.message}</div>}
          <div className="kv">
            <div className="k">Phase</div><div className="v">{node.phase}</div>
            <div className="k">Started</div><div className="v">{node.startedAt ?? '—'}</div>
            <div className="k">Finished</div><div className="v">{node.finishedAt ?? '—'}</div>
            <div className="k">Duration</div><div className="v">{duration(node.startedAt, node.finishedAt)}</div>
            {node.outputs?.exitCode && <><div className="k">Exit code</div><div className="v">{node.outputs.exitCode}</div></>}
          </div>
          {(node.inputs?.parameters?.length ?? 0) > 0 && <><h4 style={{ marginTop: 14 }}>Inputs</h4><div className="kv">{kv(node.inputs?.parameters)}</div></>}
          {(node.outputs?.parameters?.length ?? 0) > 0 && <><h4 style={{ marginTop: 14 }}>Outputs</h4><div className="kv">{kv(node.outputs?.parameters)}</div></>}
          {(node.outputs?.artifacts?.length ?? 0) > 0 && <div className="muted-sm" style={{ marginTop: 8 }}>Artifacts: {node.outputs!.artifacts!.map((a) => a.name).join(', ')}</div>}
        </>
      )}
      {tab === 'logs' && <PodsAndLogs objKey={objKey} preferNodeId={node.id} preferContainer="main" />}
    </div>
  )
}

function WfParams({ raw }: { raw: Record<string, any> | null }) { // eslint-disable-line @typescript-eslint/no-explicit-any
  if (!raw) return <div className="help">Loading…</div>
  const ps: { name: string; value?: string }[] = raw.spec?.arguments?.parameters ?? []
  return ps.length ? (
    <div className="kv">{ps.map((p) => <Fragment key={p.name}><div className="k">{p.name}</div><div className="v mono">{p.value ?? ''}</div></Fragment>)}</div>
  ) : <div className="muted-sm">No parameters.</div>
}

// ---- template & cron pages ----------------------------------------------------------------

export function TemplatePage({ objKey, left, onClose, notify, onOpenKey }: { objKey: string; left: number; onClose: () => void; notify: Notify; onOpenKey: (k: string) => void }) {
  const data = useKData()
  const o = getK(objKey)
  const params = (o?.fields.params as { name: string; value: string; description?: string; enum?: string[] }[]) ?? []
  const [vals, setVals] = useState<Record<string, string>>(() => Object.fromEntries(params.map((p) => [p.name, p.value ?? ''])))
  const [busy, setBusy] = useState(false)
  if (!o) return null
  const runs = [...data.objs.values()].filter((w) => w.kind === 'Workflow' && w.ctx === o.ctx && w.fields.template === o.name)
    .sort((a, b) => String(b.fields.startedAt || b.created).localeCompare(String(a.fields.startedAt || a.created)))
  const submit = async () => {
    setBusy(true)
    try {
      const k = await API.SubmitTemplate(objKey, vals)
      notify(`Submitted ${k.split('/').pop()}`, true)
      onOpenKey(k)
    } catch (e) {
      notify(`Submit failed: ${e}`, false)
    }
    setBusy(false)
  }
  return (
    <KPage obj={o} left={left} onClose={onClose} icon="wft" subtitle={<>entrypoint <b>{String(o.fields.entrypoint ?? '')}</b></>}
      tabs={[
        {
          id: 'submit', label: 'Submit',
          render: () => (
            <div className="card" style={{ padding: 16, maxWidth: 640 }}>
              {params.length === 0 && <div className="muted-sm" style={{ marginBottom: 10 }}>This template has no parameters.</div>}
              {params.map((p) => (
                <div key={p.name} className="field" style={{ marginBottom: 10 }}>
                  <label>{p.name}{p.description ? ` — ${p.description}` : ''}</label>
                  {p.enum?.length ? (
                    <select value={vals[p.name]} onChange={(e) => setVals({ ...vals, [p.name]: e.target.value })}>
                      {p.enum.map((v) => <option key={v}>{v}</option>)}
                    </select>
                  ) : (
                    <input type="text" className="mono" value={vals[p.name] ?? ''} onChange={(e) => setVals({ ...vals, [p.name]: e.target.value })} />
                  )}
                </div>
              ))}
              <button className="btn primary" disabled={busy} onClick={submit}>{busy ? 'Submitting…' : '▶ Submit workflow'}</button>
            </div>
          ),
        },
        {
          id: 'runs', label: <>Runs <span className="badge">{runs.length}</span></>,
          render: () => (
            <table className="grid">
              <tbody>
                {runs.map((w) => (
                  <tr key={w.key} className="click" onClick={() => onOpenKey(w.key)}>
                    <td style={{ width: 24 }}><PhaseIcon kind="Workflow" phase={w.phase} /></td>
                    <td><b>{w.name}</b></td>
                    <td className="muted-sm">{ago(String(w.fields.startedAt || w.created))} ago</td>
                    <td><Problem1 o={w} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          ),
        },
      ]}
    />
  )
}

export function CronPage({ objKey, left, onClose, notify, onOpenKey }: { objKey: string; left: number; onClose: () => void; notify: Notify; onOpenKey: (k: string) => void }) {
  const data = useKData()
  const o = getK(objKey)
  const run = useAction(notify)
  if (!o) return null
  const runs = [...data.objs.values()].filter((w) => w.kind === 'Workflow' && w.ctx === o.ctx && w.fields.cron === o.name)
    .sort((a, b) => String(b.fields.startedAt || b.created).localeCompare(String(a.fields.startedAt || a.created)))
  const submit = async () => {
    try {
      const k = await API.CronAction(objKey, 'submit')
      notify('Submitted', true)
      onOpenKey(k)
    } catch (e) {
      notify(`Submit failed: ${e}`, false)
    }
  }
  return (
    <KPage obj={o} left={left} onClose={onClose} icon="cron"
      subtitle={<><span className="mono">{String(o.fields.schedule)}</span> {String(o.fields.timezone ?? '')} · last run {o.fields.lastScheduled ? ago(String(o.fields.lastScheduled)) + ' ago' : 'never'}</>}
      actions={
        <>
          <button className="btn primary" onClick={submit}>▶ Run now</button>
          {o.fields.suspended
            ? <button className="btn" onClick={() => run('Resume', () => API.CronAction(objKey, 'resume'))}>▶ Resume schedule</button>
            : <button className="btn" onClick={() => run('Suspend', () => API.CronAction(objKey, 'suspend'))}>❚❚ Suspend schedule</button>}
          <span className="spacer" />
          <ConfirmButton className="danger-outline" label="🗑 Delete" confirm={`Delete ${o.name}?`} onConfirm={() => run('Delete', () => API.KDelete(objKey)).then(onClose)} />
        </>
      }
      tabs={[{
        id: 'runs', label: <>Runs <span className="badge">{runs.length}</span></>,
        render: () => (
          <table className="grid">
            <tbody>
              {runs.map((w) => (
                <tr key={w.key} className="click" onClick={() => onOpenKey(w.key)}>
                  <td style={{ width: 24 }}><PhaseIcon kind="Workflow" phase={w.phase} /></td>
                  <td><b>{w.name}</b></td>
                  <td className="muted-sm">{ago(String(w.fields.startedAt || w.created))} ago</td>
                  <td><Problem1 o={w} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        ),
      }]}
    />
  )
}
