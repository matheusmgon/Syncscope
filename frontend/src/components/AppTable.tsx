import { useEffect, useMemo, useRef, useState } from 'react'
import type { App } from '../data'
import type { SortKey } from '../search'
import { HealthIcon, StatBar, SyncIcon, ago } from './Status'

export type GroupBy = 'none' | 'appset' | 'cluster' | 'project' | 'ctx'

const ROW = 34

type Row = { type: 'group'; id: string; label: string; apps: App[] } | { type: 'app'; app: App }

type Props = {
  apps: App[]
  groupBy: GroupBy
  ctxNames: Map<string, string>
  selected: Set<string>
  setSelected: (s: Set<string>) => void
  focused: string | null
  onOpen: (key: string) => void
  onAddFilter: (term: string) => void
  sort: { key: SortKey; desc: boolean }
  setSort: (s: { key: SortKey; desc: boolean }) => void
}

function groupKey(a: App, g: GroupBy, ctxNames: Map<string, string>) {
  switch (g) {
    case 'appset': return a.appSet || '(no ApplicationSet)'
    case 'cluster': return a.cluster || '(no cluster)'
    case 'project': return a.project
    case 'ctx': return ctxNames.get(a.ctx) ?? a.ctx
  }
  return ''
}

function q(v: string) {
  return /\s/.test(v) ? `"${v}"` : v
}

export function AppTable(p: Props) {
  const body = useRef<HTMLDivElement>(null)
  const [scroll, setScroll] = useState(0)
  const [height, setHeight] = useState(800)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const lastClicked = useRef<number | null>(null)

  useEffect(() => {
    const el = body.current
    if (!el) return
    const ro = new ResizeObserver(() => setHeight(el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const rows: Row[] = useMemo(() => {
    if (p.groupBy === 'none') return p.apps.map((app) => ({ type: 'app', app }))
    const groups = new Map<string, App[]>()
    for (const a of p.apps) {
      const k = groupKey(a, p.groupBy, p.ctxNames)
      let g = groups.get(k)
      if (!g) groups.set(k, (g = []))
      g.push(a)
    }
    const sorted = [...groups.entries()].sort((a, b) => {
      const ea = a[1].filter((x) => x.severity === 2).length
      const eb = b[1].filter((x) => x.severity === 2).length
      return (eb > 0 ? 1 : 0) - (ea > 0 ? 1 : 0) || a[0].localeCompare(b[0])
    })
    const out: Row[] = []
    for (const [k, apps] of sorted) {
      out.push({ type: 'group', id: k, label: k, apps })
      if (!collapsed.has(k)) for (const app of apps) out.push({ type: 'app', app })
    }
    return out
  }, [p.apps, p.groupBy, p.ctxNames, collapsed])

  const appRows = useMemo(() => rows.flatMap((r) => (r.type === 'app' ? [r.app] : [])), [rows])

  // keep focused row visible
  useEffect(() => {
    if (!p.focused || !body.current) return
    const i = rows.findIndex((r) => r.type === 'app' && r.app.key === p.focused)
    if (i < 0) return
    const el = body.current
    if (i * ROW < el.scrollTop) el.scrollTop = i * ROW
    else if ((i + 1) * ROW > el.scrollTop + el.clientHeight) el.scrollTop = (i + 1) * ROW - el.clientHeight
  }, [p.focused, rows])

  const start = Math.max(0, Math.floor(scroll / ROW) - 10)
  const end = Math.min(rows.length, Math.ceil((scroll + height) / ROW) + 10)

  const toggle = (a: App, e: React.MouseEvent) => {
    const s = new Set(p.selected)
    const idx = appRows.indexOf(a)
    if (e.shiftKey && lastClicked.current !== null) {
      const [from, to] = [Math.min(lastClicked.current, idx), Math.max(lastClicked.current, idx)]
      for (let i = from; i <= to; i++) s.add(appRows[i].key)
    } else if (s.has(a.key)) s.delete(a.key)
    else s.add(a.key)
    lastClicked.current = idx
    p.setSelected(s)
  }

  const allSelected = p.apps.length > 0 && p.apps.every((a) => p.selected.has(a.key))
  const header = (key: SortKey | null, label: string) =>
    key ? (
      <div className="sortable" onClick={() => p.setSort({ key, desc: p.sort.key === key ? !p.sort.desc : false })}>
        {label} {p.sort.key === key ? (p.sort.desc ? '▾' : '▴') : ''}
      </div>
    ) : (
      <div>{label}</div>
    )

  return (
    <div className="table">
      <div className="thead">
        <div>
          <input
            type="checkbox"
            checked={allSelected}
            onChange={() => p.setSelected(allSelected ? new Set() : new Set(p.apps.map((a) => a.key)))}
            title="Select all results"
          />
        </div>
        {header('health', 'H')}
        {header('sync', 'S')}
        {header('name', 'Application')}
        {header('appset', 'ApplicationSet')}
        {header('project', 'Project')}
        {header('cluster', 'Cluster / namespace')}
        <div>Revision</div>
        {header('severity', 'Problem')}
        {header('synced', 'Last sync')}
      </div>
      <div className="tbody" ref={body} onScroll={(e) => setScroll((e.target as HTMLDivElement).scrollTop)}>
        <div style={{ height: rows.length * ROW, position: 'relative' }}>
          {rows.slice(start, end).map((r, i) => {
            const top = (start + i) * ROW
            if (r.type === 'group') {
              const hc: Record<string, number> = {}
              let err = 0, oos = 0
              for (const a of r.apps) {
                hc[a.health] = (hc[a.health] ?? 0) + 1
                if (a.severity === 2) err++
                if (a.sync === 'OutOfSync') oos++
              }
              const all = r.apps.every((a) => p.selected.has(a.key))
              const isCollapsed = collapsed.has(r.id)
              return (
                <div
                  key={'g:' + r.id}
                  className="grouprow"
                  style={{ top }}
                  onClick={() => {
                    const c = new Set(collapsed)
                    if (isCollapsed) c.delete(r.id)
                    else c.add(r.id)
                    setCollapsed(c)
                  }}
                >
                  <input
                    type="checkbox"
                    checked={all}
                    onClick={(e) => e.stopPropagation()}
                    onChange={() => {
                      const s = new Set(p.selected)
                      for (const a of r.apps) all ? s.delete(a.key) : s.add(a.key)
                      p.setSelected(s)
                    }}
                    title="Select group"
                  />
                  <span className={'caret' + (isCollapsed ? ' closed' : '')}>▾</span>
                  <span>{r.label}</span>
                  <span className="badge">{r.apps.length}</span>
                  {err > 0 && <span className="badge error">{err} failing</span>}
                  {oos > 0 && <span className="badge warning">{oos} OutOfSync</span>}
                  <span className="spacer" />
                  <StatBar
                    total={r.apps.length}
                    counts={['Healthy', 'Progressing', 'Suspended', 'Missing', 'Degraded', 'Unknown'].map((k) => [k, hc[k] ?? 0])}
                  />
                </div>
              )
            }
            const a = r.app
            const prob = a.problems?.[0]
            return (
              <div
                key={a.key}
                className={
                  'trow' + (p.selected.has(a.key) ? ' selected' : '') + (p.focused === a.key ? ' focused' : '') +
                  (a.severity === 2 ? ' sev-2' : '')
                }
                style={{ top }}
                onClick={(e) => (e.metaKey || e.ctrlKey || e.shiftKey ? toggle(a, e) : p.onOpen(a.key))}
              >
                <div onClick={(e) => { e.stopPropagation(); toggle(a, e) }}>
                  <input type="checkbox" checked={p.selected.has(a.key)} readOnly />
                </div>
                <HealthIcon status={a.health} />
                <SyncIcon status={a.sync} running={a.opPhase === 'Running'} />
                <div className="cell name" title={a.name}>
                  {a.name}
                  {p.groupBy !== 'ctx' && p.ctxNames.size > 1 && <span className="sub">{p.ctxNames.get(a.ctx)}</span>}
                </div>
                <div className="cell muted">
                  {a.appSet ? (
                    <span className="link" onClick={(e) => { e.stopPropagation(); p.onAddFilter('appset:' + q(a.appSet)) }}>{a.appSet}</span>
                  ) : '—'}
                </div>
                <div className="cell muted">
                  <span className="link" onClick={(e) => { e.stopPropagation(); p.onAddFilter('project:' + q(a.project)) }}>{a.project}</span>
                </div>
                <div className="cell muted" title={a.clusterServer}>
                  <span className="link" onClick={(e) => { e.stopPropagation(); p.onAddFilter('cluster:' + q(a.cluster)) }}>{a.cluster}</span>
                  <span className="sub">{a.destNamespace}</span>
                </div>
                <div className="cell muted mono" title={`${a.targetRev} → ${a.syncRev}`}>{a.syncRev || a.targetRev}</div>
                <div className={'cell problem' + (prob?.severity === 'warning' ? ' warn' : '')} title={a.problems?.map((x) => (x.resource ? x.resource + ': ' : '') + x.message).join('\n')}>
                  {prob && (
                    <>
                      {prob.resource && <span className="res">{prob.resource}</span>}
                      {prob.message}
                      {a.problems!.length > 1 && <span className="sub">+{a.problems!.length - 1}</span>}
                    </>
                  )}
                </div>
                <div className="cell muted" title={a.opFinishedAt}>
                  {a.opPhase === 'Running' ? 'syncing…' : ago(a.opFinishedAt)}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
