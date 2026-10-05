import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { kube, kubestore, store } from '../../../wailsjs/go/models'
import * as API from '../../../wailsjs/go/main/App'
import { HealthIcon, ago } from '../Status'
import { LogStream, type LogSource } from '../LogViewer'
import { EventsView } from '../EventsView'
import { YamlEditor } from '../YamlEditor'
import { ProblemList } from '../Problems'
import type { KObj } from '../../kdata'

// ---- phases ---------------------------------------------------------------------------

// Map each tool's phase vocabulary onto the Argo CD health icons.
export function phaseStatus(kind: string, phase: string): string {
  switch (phase) {
    case 'Succeeded': case 'Healthy': case 'Active': case 'Ready': return 'Healthy'
    case 'Running': return kind === 'Workflow' ? 'Progressing' : 'Healthy'
    case 'Failed': case 'Error': case 'Degraded': return 'Degraded'
    case 'Progressing': return 'Progressing'
    case 'Paused': case 'Suspended': return 'Suspended'
    case 'Pending': return 'Missing'
  }
  return 'Unknown'
}

export function PhaseIcon({ kind, phase }: { kind: string; phase: string }) {
  return <span title={phase}><HealthIcon status={phaseStatus(kind, phase)} /></span>
}

export function PhasePill({ kind, phase }: { kind: string; phase: string }) {
  return <span className="pill"><HealthIcon status={phaseStatus(kind, phase)} /> {phase}</span>
}

export function duration(from?: string, to?: string) {
  if (!from) return ''
  const a = new Date(from).getTime()
  const b = to ? new Date(to).getTime() : Date.now()
  if (isNaN(a) || isNaN(b)) return ''
  let s = Math.max(0, Math.round((b - a) / 1000))
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  s -= m * 60
  return h ? `${h}h${m}m` : m ? `${m}m${s}s` : `${s}s`
}

// ---- virtualized table ------------------------------------------------------------------

export type Column<T> = { header: string; width: string; render: (row: T) => ReactNode; title?: (row: T) => string }

const ROW = 34

export function VTable<T>({ rows, columns, rowKey, onOpen, rowClass, empty }: {
  rows: T[]
  columns: Column<T>[]
  rowKey: (r: T) => string
  onOpen?: (r: T) => void
  rowClass?: (r: T) => string
  empty?: ReactNode
}) {
  const body = useRef<HTMLDivElement>(null)
  const [scroll, setScroll] = useState(0)
  const [height, setHeight] = useState(800)
  useEffect(() => {
    const el = body.current
    if (!el) return
    const ro = new ResizeObserver(() => setHeight(el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const grid = columns.map((c) => c.width).join(' ')
  const start = Math.max(0, Math.floor(scroll / ROW) - 10)
  const end = Math.min(rows.length, Math.ceil((scroll + height) / ROW) + 10)
  return (
    <div className="table">
      <div className="thead" style={{ gridTemplateColumns: grid }}>
        {columns.map((c, i) => <div key={i}>{c.header}</div>)}
      </div>
      <div className="tbody" ref={body} onScroll={(e) => setScroll((e.target as HTMLDivElement).scrollTop)}>
        {rows.length === 0 && (empty ?? <div className="empty">Nothing here.</div>)}
        <div style={{ height: rows.length * ROW, position: 'relative' }}>
          {rows.slice(start, end).map((r, i) => (
            <div
              key={rowKey(r)}
              className={'trow ' + (rowClass?.(r) ?? '')}
              style={{ top: (start + i) * ROW, gridTemplateColumns: grid }}
              onClick={() => onOpen?.(r)}
            >
              {columns.map((c, j) => (
                <div key={j} className="cell" title={c.title?.(r)}>{c.render(r)}</div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ---- pods & logs ------------------------------------------------------------------------

export function PodsAndLogs({ objKey, preferPod, preferNodeId, preferContainer }: { objKey: string; preferPod?: string; preferNodeId?: string; preferContainer?: string }) {
  const [pods, setPods] = useState<kube.PodInfo[] | null>(null)
  const [err, setErr] = useState('')
  const [sel, setSel] = useState<string>('')
  const ctx = objKey.slice(0, objKey.indexOf('|'))

  const load = useCallback(() => {
    API.KPods(objKey)
      .then((p) => {
        setPods(p ?? [])
        const byNode = preferNodeId ? (p ?? []).find((x) => x.annotations?.['workflows.argoproj.io/node-id'] === preferNodeId)?.name : undefined
        setSel((cur) => byNode || cur || preferPod || (p ?? []).find((x) => x.reason)?.name || p?.[0]?.name || '')
      })
      .catch((e) => setErr(String(e)))
  }, [objKey, preferPod, preferNodeId])
  useEffect(load, [load])
  useEffect(() => { if (preferPod) setSel(preferPod) }, [preferPod])

  const pod = pods?.find((p) => p.name === sel)
  const source = useMemo<LogSource | null>(() => {
    if (!pod) return null
    return {
      id: ctx + '|' + pod.namespace + '/' + pod.name,
      aggregated: false,
      containers: async () => {
        const all = [...(pod.containers ?? []), ...(pod.init ?? [])]
        if (preferContainer && all.includes(preferContainer)) return [preferContainer, ...all.filter((c) => c !== preferContainer)]
        // Argo Workflows pods: "main" holds the user's container; "wait" is the executor
        return all.includes('main') ? ['main', ...all.filter((c) => c !== 'main')] : all
      },
      start: (o) => API.KStartLogs(kubestore.LogRequest.createFrom({ ctx, namespace: pod.namespace, pod: pod.name, ...o })),
    }
  }, [pod, ctx, preferContainer])

  if (err) return <div className="alert">{err}</div>
  if (!pods) return <div className="help">Loading pods…</div>
  if (!pods.length) return <div className="help">No pods found for this object.</div>
  return (
    <div className="pods-logs">
      <div className="pods-list">
        {pods.map((p) => (
          <div key={p.name} className={'pod-row' + (p.name === sel ? ' on' : '')} onClick={() => setSel(p.name)}>
            <HealthIcon status={p.reason || p.phase === 'Failed' ? 'Degraded' : p.phase === 'Running' || p.phase === 'Succeeded' ? 'Healthy' : 'Progressing'} />
            <div style={{ minWidth: 0 }}>
              <div className="mono" style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{p.name}</div>
              <div className="muted-sm">{p.phase}{p.reason ? ` · ${p.reason}` : ''} · {p.ready} ready{p.restarts ? ` · ${p.restarts} restarts` : ''} · {ago(p.created)}</div>
            </div>
          </div>
        ))}
        <button className="btn sm" style={{ margin: 8 }} onClick={load}>Reload pods</button>
      </div>
      <div className="pods-log">{source ? <LogStream key={source.id} source={source} /> : <div className="help">Select a pod.</div>}</div>
    </div>
  )
}

// ---- detail page frame ------------------------------------------------------------------

export type PageTab = { id: string; label: ReactNode; render: () => ReactNode; flush?: boolean }

export function KPage({ obj, left, onClose, icon, actions, tabs, subtitle }: {
  obj: KObj
  left: number
  onClose: () => void
  icon: string
  actions?: ReactNode
  tabs: PageTab[]
  subtitle?: ReactNode
}) {
  const yamlTab: PageTab = { id: 'yaml', label: 'YAML', render: () => <YamlTab objKey={obj.key} /> }
  const eventsTab: PageTab = { id: 'events', label: 'Events', render: () => <KEventsTab objKey={obj.key} /> }
  const all = [...tabs, eventsTab, yamlTab]
  const [tab, setTab] = useState(all[0].id)
  const [showProblems, setShowProblems] = useState(true)
  const current = all.find((t) => t.id === tab) ?? all[0]
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && !document.querySelector('.modal-backdrop') && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])
  const problems = (obj.problems ?? []) as unknown as store.Problem[]
  return (
    <div className="drawer page" style={{ left }}>
      <header>
        <h2>
          <span className="kicon" style={{ width: 28, height: 28, fontSize: 8 }}>{icon}</span>
          <span className="selectable">{obj.name}</span>
          <PhasePill kind={obj.kind} phase={obj.phase} />
          <button className="btn ghost sm back" onClick={onClose}>← Back</button>
        </h2>
        <div className="meta">{obj.kind} · {obj.ctx} · {obj.namespace}{subtitle ? <> · {subtitle}</> : null}</div>
        {actions && <div className="tools">{actions}</div>}
        <div className="tabs detail-tabs">
          {all.map((t) => (
            <div key={t.id} className={'tab' + (t.id === current.id ? ' active' : '')} onClick={() => setTab(t.id)}>{t.label}</div>
          ))}
        </div>
      </header>
      {problems.length > 0 && (
        <div className="detail-problems selectable">
          <div className="dp-head" onClick={() => setShowProblems(!showProblems)}>
            <b>Why it is failing</b> <span className={'badge ' + (obj.severity === 2 ? 'error' : 'warning')}>{problems.length}</span>
            <span className="spacer" /><span className="muted-sm">{showProblems ? 'hide' : 'show'}</span>
          </div>
          {showProblems && <ProblemList problems={problems} />}
        </div>
      )}
      <div className={current.flush ? 'body tree-body' : 'body selectable'}>{current.render()}</div>
    </div>
  )
}

function YamlTab({ objKey }: { objKey: string }) {
  const load = useCallback(() => API.KYAML(objKey), [objKey])
  const save = useCallback((y: string) => API.KSaveYAML(objKey, y), [objKey])
  return <YamlEditor load={load} save={save} />
}

function KEventsTab({ objKey }: { objKey: string }) {
  const load = useCallback(
    () => API.KEvents(objKey).then((evs) => (evs ?? []).map((e) => store.EventRow.createFrom(e))),
    [objKey],
  )
  return <EventsView load={load} />
}

// ---- small helpers ------------------------------------------------------------------------

export function useAction(notify: (m: string, ok: boolean) => void) {
  return async (label: string, fn: () => Promise<unknown>) => {
    try {
      await fn()
      notify(`${label}: done`, true)
    } catch (e) {
      notify(`${label} failed: ${e}`, false)
    }
  }
}

export function ConfirmButton({ label, confirm, onConfirm, className }: { label: ReactNode; confirm: string; onConfirm: () => void; className?: string }) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const t = setTimeout(() => setArmed(false), 4000)
    return () => clearTimeout(t)
  }, [armed])
  return armed ? (
    <button className="btn danger" onClick={() => { setArmed(false); onConfirm() }}>{confirm}</button>
  ) : (
    <button className={'btn ' + (className ?? '')} onClick={() => setArmed(true)}>{label}</button>
  )
}

export function Problem1({ o }: { o: KObj }) {
  const p = o.problems?.[0]
  if (!p) return null
  return (
    <span className={'problem' + (p.severity === 'warning' ? ' warn' : '')} style={{ color: p.severity === 'warning' ? 'var(--warning-fg)' : 'var(--error-fg)' }}
      title={(o.problems ?? []).map((x) => (x.resource ? x.resource + ': ' : '') + x.message).join('\n')}>
      {p.resource && <b>{p.resource}: </b>}{p.message}
    </span>
  )
}
