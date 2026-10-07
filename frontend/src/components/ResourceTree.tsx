import { useEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent } from 'react'
import { store } from '../../wailsjs/go/models'
import { HealthIcon, SyncIcon, ago } from './Status'
import { ResourcePanel, kindAbbr, type PanelTab } from './ResourcePanel'

// Argo CD-style resource graph: application on the left, owned resources fanning
// out to the right (Deployment → ReplicaSet → Pod), connected by curved edges.

const NODE_W = 250
const NODE_H = 52
const COL_GAP = 70
const ROW_GAP = 12
const APP_ID = '__app'


const kindOrder = [
  'Namespace', 'CustomResourceDefinition', 'ServiceAccount', 'Role', 'ClusterRole', 'RoleBinding', 'ClusterRoleBinding',
  'Secret', 'ExternalSecret', 'ConfigMap', 'PersistentVolumeClaim', 'Service', 'Deployment', 'Rollout', 'StatefulSet',
  'DaemonSet', 'ReplicaSet', 'Job', 'CronJob', 'Pod', 'Ingress',
]
const kindRank = (k: string) => {
  const i = kindOrder.indexOf(k)
  return i < 0 ? kindOrder.length : i
}

const isBad = (n: store.TreeNode) =>
  n.health === 'Degraded' || n.health === 'Missing' || n.health === 'Progressing' || n.sync === 'OutOfSync'

type Placed = { node: store.TreeNode | null; id: string; x: number; y: number; hidden: number }

type Props = {
  appKey: string
  selfHeal: boolean
  notify: (m: string, ok: boolean) => void
  app: { name: string; health: string; sync: string; opPhase?: string; syncRev: string }
  nodes: store.TreeNode[]
}

export function ResourceTree({ appKey, app, nodes, selfHeal, notify }: Props) {
  const [sideTab, setSideTab] = useState<PanelTab>('details')
  const [zoom, setZoom] = useState(1)
  const [filter, setFilter] = useState('')
  const [onlyBad, setOnlyBad] = useState(false)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [selected, setSelected] = useState<string | null>(null)
  const scroller = useRef<HTMLDivElement>(null)

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes])

  // children map, with parentless nodes hanging off the application node
  const children = useMemo(() => {
    const m = new Map<string, string[]>()
    for (const n of nodes) {
      const ps = n.parents?.length ? n.parents : [APP_ID]
      // a node with several owners is drawn once, under its first owner
      const p = ps[0]
      if (!m.has(p)) m.set(p, [])
      m.get(p)!.push(n.id)
    }
    for (const [, list] of m) {
      list.sort((a, b) => {
        const na = byId.get(a)!, nb = byId.get(b)!
        return kindRank(na.kind) - kindRank(nb.kind) || na.kind.localeCompare(nb.kind) || na.name.localeCompare(nb.name)
      })
    }
    return m
  }, [nodes, byId])

  // nodes visible under the current filter: matches plus all their ancestors
  const visible = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q && !onlyBad) return null
    const keep = new Set<string>()
    for (const n of nodes) {
      const match = (!q || `${n.kind} ${n.name} ${n.namespace}`.toLowerCase().includes(q)) && (!onlyBad || isBad(n))
      if (!match) continue
      let cur: store.TreeNode | undefined = n
      while (cur && !keep.has(cur.id)) {
        keep.add(cur.id)
        cur = cur.parents?.length ? byId.get(cur.parents[0]) : undefined
      }
    }
    return keep
  }, [nodes, byId, filter, onlyBad])

  const layout = useMemo(() => {
    const placed: Placed[] = []
    const edges: { from: Placed; to: Placed }[] = []
    let nextY = 0
    const visit = (id: string, depth: number): Placed => {
      const kids = (children.get(id) ?? []).filter((k) => !visible || visible.has(k))
      const isCollapsed = collapsed.has(id)
      const shown = isCollapsed ? [] : kids
      const p: Placed = { node: id === APP_ID ? null : byId.get(id)!, id, x: depth * (NODE_W + COL_GAP), y: 0, hidden: isCollapsed ? kids.length : 0 }
      if (!shown.length) {
        p.y = nextY
        nextY += NODE_H + ROW_GAP
      } else {
        const placedKids = shown.map((k) => visit(k, depth + 1))
        p.y = (placedKids[0].y + placedKids[placedKids.length - 1].y) / 2
        for (const c of placedKids) edges.push({ from: p, to: c })
      }
      placed.push(p)
      return p
    }
    visit(APP_ID, 0)
    const width = Math.max(...placed.map((p) => p.x)) + NODE_W + 20
    return { placed, edges, width, height: Math.max(nextY, NODE_H) + 20 }
  }, [children, byId, collapsed, visible])

  // ⌘/ctrl + wheel zooms, like the Argo CD UI
  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const h = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return
      e.preventDefault()
      setZoom((z) => Math.min(2, Math.max(0.3, z * (e.deltaY > 0 ? 0.9 : 1.1))))
    }
    el.addEventListener('wheel', h, { passive: false })
    return () => el.removeEventListener('wheel', h)
  }, [])

  // drag on empty canvas space pans the view
  const drag = useRef<{ x: number; y: number; sl: number; st: number; moved: boolean } | null>(null)
  const [panning, setPanning] = useState(false)
  const onPanStart = (e: ReactMouseEvent) => {
    const el = scroller.current
    if (!el || e.button !== 0 || (e.target as HTMLElement).closest('.rnode, button')) return
    drag.current = { x: e.clientX, y: e.clientY, sl: el.scrollLeft, st: el.scrollTop, moved: false }
    const move = (ev: MouseEvent) => {
      const d = drag.current
      if (!d) return
      const dx = ev.clientX - d.x, dy = ev.clientY - d.y
      if (!d.moved && Math.abs(dx) + Math.abs(dy) < 4) return
      if (!d.moved) { d.moved = true; setPanning(true) }
      el.scrollLeft = d.sl - dx
      el.scrollTop = d.st - dy
    }
    const up = () => {
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', up)
      setPanning(false)
      // let the click that follows see the drag, then forget it
      setTimeout(() => { drag.current = null }, 0)
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
    e.preventDefault()
  }

  const fit = () => {
    const el = scroller.current
    if (!el) return
    setZoom(Math.min(1, Math.max(0.3, Math.min((el.clientWidth - 20) / layout.width, (el.clientHeight - 20) / layout.height))))
  }

  const sel = selected ? byId.get(selected) : undefined
  const badCount = nodes.filter(isBad).length

  return (
    <div className="rtree">
      <div className="rtree-toolbar">
        <input className="rtree-filter" placeholder="Filter resources…" value={filter} onChange={(e) => setFilter(e.target.value)} spellCheck={false} />
        <label className="check"><input type="checkbox" checked={onlyBad} onChange={(e) => setOnlyBad(e.target.checked)} /> only unhealthy / out of sync ({badCount})</label>
        <span className="spacer" />
        <span className="muted-sm">{nodes.length} resources</span>
        <button className="btn sm" onClick={() => setCollapsed(new Set())}>Expand all</button>
        <button className="btn sm" onClick={() => setZoom((z) => Math.max(0.3, z - 0.1))}>−</button>
        <span className="muted-sm" style={{ width: 40, textAlign: 'center' }}>{Math.round(zoom * 100)}%</span>
        <button className="btn sm" onClick={() => setZoom((z) => Math.min(2, z + 0.1))}>＋</button>
        <button className="btn sm" onClick={fit}>Fit</button>
      </div>
      <div className="rtree-main">
        <div
          className={'rtree-canvas' + (panning ? ' panning' : '')}
          ref={scroller}
          onMouseDown={onPanStart}
          onClick={() => { if (!drag.current?.moved) setSelected(null) }}
        >
          <div style={{ width: layout.width * zoom, height: layout.height * zoom, position: 'relative' }}>
            <div style={{ transform: `scale(${zoom})`, transformOrigin: '0 0', width: layout.width, height: layout.height, position: 'absolute' }}>
              <svg width={layout.width} height={layout.height} className="rtree-edges">
                {layout.edges.map(({ from, to }) => {
                  const x1 = from.x + NODE_W, y1 = from.y + NODE_H / 2 + 10
                  const x2 = to.x, y2 = to.y + NODE_H / 2 + 10
                  const mx = (x1 + x2) / 2
                  const bad = to.node && (to.node.health === 'Degraded' || to.node.health === 'Missing')
                  return <path key={from.id + '>' + to.id} d={`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} className={bad ? 'bad' : ''} />
                })}
              </svg>
              {layout.placed.map((p) => {
                const kidCount = (children.get(p.id) ?? []).filter((k) => !visible || visible.has(k)).length
                const toggle = kidCount > 0 && (
                  <button
                    className="rnode-toggle"
                    title={collapsed.has(p.id) ? 'Expand' : 'Collapse'}
                    onClick={(e) => {
                      e.stopPropagation()
                      const c = new Set(collapsed)
                      c.has(p.id) ? c.delete(p.id) : c.add(p.id)
                      setCollapsed(c)
                    }}
                  >
                    {collapsed.has(p.id) ? `+${p.hidden}` : '−'}
                  </button>
                )
                if (!p.node) {
                  return (
                    <div key={p.id} className="rnode app" style={{ left: p.x, top: p.y + 10 }}>
                      <span className="kicon app">app</span>
                      <div className="rnode-text">
                        <div className="rnode-name" title={app.name}>{app.name}</div>
                        <div className="rnode-kind">Application{app.syncRev ? ` · ${app.syncRev}` : ''}</div>
                      </div>
                      <div className="rnode-status"><HealthIcon status={app.health} /><SyncIcon status={app.sync} running={app.opPhase === 'Running'} /></div>
                      {toggle}
                    </div>
                  )
                }
                const n = p.node
                const sev = n.health === 'Degraded' ? ' degraded' : n.health === 'Missing' ? ' missing' : n.sync === 'OutOfSync' ? ' oos' : ''
                return (
                  <div
                    key={p.id}
                    className={'rnode' + sev + (selected === n.id ? ' selected' : '') + (n.managed ? '' : ' child')}
                    style={{ left: p.x, top: p.y + 10 }}
                    onClick={(e) => { e.stopPropagation(); setSelected(n.id);  }}
                    onDoubleClick={(e) => { e.stopPropagation(); if (n.hasLogs) { setSelected(n.id); setSideTab('logs') } }}
                    title={n.healthMsg || undefined}
                  >
                    <span className="kicon">{kindAbbr[n.kind] ?? n.kind.slice(0, 4).toLowerCase()}</span>
                    <div className="rnode-text">
                      <div className="rnode-name">{n.name}</div>
                      <div className={'rnode-kind' + (n.healthMsg && n.health !== 'Healthy' ? ' err' : '')}>
                        {n.healthMsg && n.health !== 'Healthy' ? n.healthMsg : <>{n.kind}{n.info?.Revision ? ` · ${n.info.Revision}` : ''}{n.kind === 'Pod' && n.info?.Containers ? ` · ${n.info.Containers}` : ''}{n.createdAt ? ` · ${ago(n.createdAt)}` : ''}</>}
                      </div>
                    </div>
                    <div className="rnode-status">
                      {n.health && <HealthIcon status={n.health} />}
                      {n.managed && n.sync && <SyncIcon status={n.sync} />}
                    </div>
                    {toggle}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
        {sel && (
          <ResourcePanel
            appKey={appKey}
            node={sel}
            tab={sideTab}
            setTab={setSideTab}
            selfHeal={selfHeal}
            onClose={() => setSelected(null)}
            notify={notify}
          />
        )}
      </div>
    </div>
  )
}
