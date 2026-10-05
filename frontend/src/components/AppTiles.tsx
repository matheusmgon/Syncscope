import { useEffect, useRef, useState } from 'react'
import type { App } from '../data'
import { HealthIcon, SyncIcon, ago } from './Status'

// Card grid like the Argo CD applications page, virtualized by row.

const TILE_W = 320
const TILE_H = 196
const GAP = 12

type Props = {
  apps: App[]
  ctxNames: Map<string, string>
  selected: Set<string>
  setSelected: (s: Set<string>) => void
  focused: string | null
  onOpen: (key: string) => void
  onAction: (action: 'sync' | 'refresh' | 'restart', keys: string[]) => void
  onAddFilter: (term: string) => void
}

export function AppTiles(p: Props) {
  const el = useRef<HTMLDivElement>(null)
  const [scroll, setScroll] = useState(0)
  const [size, setSize] = useState({ w: 1200, h: 800 })

  useEffect(() => {
    const e = el.current
    if (!e) return
    const ro = new ResizeObserver(() => setSize({ w: e.clientWidth, h: e.clientHeight }))
    ro.observe(e)
    return () => ro.disconnect()
  }, [])

  const cols = Math.max(1, Math.floor((size.w - GAP) / (TILE_W + GAP)))
  const tileW = Math.floor((size.w - GAP * (cols + 1)) / cols)
  const rows = Math.ceil(p.apps.length / cols)
  const rowH = TILE_H + GAP
  const r0 = Math.max(0, Math.floor(scroll / rowH) - 2)
  const r1 = Math.min(rows, Math.ceil((scroll + size.h) / rowH) + 2)

  const toggle = (key: string) => {
    const s = new Set(p.selected)
    s.has(key) ? s.delete(key) : s.add(key)
    p.setSelected(s)
  }

  const tiles = []
  for (let r = r0; r < r1; r++) {
    for (let c = 0; c < cols; c++) {
      const a = p.apps[r * cols + c]
      if (!a) break
      const prob = a.problems?.[0]
      tiles.push(
        <div
          key={a.key}
          className={'tile' + (p.selected.has(a.key) ? ' selected' : '') + (p.focused === a.key ? ' focused' : '') +
            (a.severity === 2 ? ' sev-2' : a.severity === 1 ? ' sev-1' : '') + ' h-' + a.health.toLowerCase()}
          style={{ left: GAP + c * (tileW + GAP), top: GAP + r * rowH, width: tileW, height: TILE_H }}
          onClick={(e) => (e.metaKey || e.ctrlKey || e.shiftKey ? toggle(a.key) : p.onOpen(a.key))}
        >
          <div className="tile-head">
            <HealthIcon status={a.health} />
            <span className="tile-name" title={a.name}>{a.name}</span>
            <input type="checkbox" checked={p.selected.has(a.key)} onClick={(e) => e.stopPropagation()} onChange={() => toggle(a.key)} />
          </div>
          <div className="tile-grid">
            <span className="k">Project</span>
            <span className="v"><span className="link" onClick={(e) => { e.stopPropagation(); p.onAddFilter(`project:"${a.project}"`) }}>{a.project}</span>{p.ctxNames.size > 1 && <span className="sub">{p.ctxNames.get(a.ctx)}</span>}</span>
            <span className="k">Status</span>
            <span className="v">
              <HealthIcon status={a.health} /> {a.health} <SyncIcon status={a.sync} running={a.opPhase === 'Running'} /> {a.sync}
            </span>
            <span className="k">ApplicationSet</span>
            <span className="v">{a.appSet ? <span className="link" onClick={(e) => { e.stopPropagation(); p.onAddFilter(`appset:"${a.appSet}"`) }}>{a.appSet}</span> : '—'}</span>
            <span className="k">Destination</span>
            <span className="v" title={a.clusterServer}>{a.cluster}/{a.destNamespace}</span>
            <span className="k">Target</span>
            <span className="v mono" title={a.repo + ' ' + a.path}>{a.path || a.repo} @ {a.targetRev || 'HEAD'}</span>
            <span className="k">Last sync</span>
            <span className="v">{a.opPhase === 'Running' ? 'syncing…' : a.opFinishedAt ? `${ago(a.opFinishedAt)} ago${a.opPhase && a.opPhase !== 'Succeeded' ? ` · ${a.opPhase}` : ''}` : '—'}</span>
          </div>
          {prob ? (
            <div className={'tile-problem' + (prob.severity === 'warning' ? ' warn' : '')} title={a.problems!.map((x) => (x.resource ? x.resource + ': ' : '') + x.message).join('\n')}>
              {prob.resource && <b>{prob.resource}: </b>}{prob.message}
              {a.problems!.length > 1 && <span className="sub"> +{a.problems!.length - 1}</span>}
            </div>
          ) : (
            <div className="tile-actions" onClick={(e) => e.stopPropagation()}>
              <button className="btn sm" onClick={() => p.onAction('sync', [a.key])}>⟳ Sync</button>
              <button className="btn sm" onClick={() => p.onAction('refresh', [a.key])}>Refresh</button>
              <button className="btn sm" disabled={!a.workloads} onClick={() => p.onAction('restart', [a.key])}>↻ Restart</button>
            </div>
          )}
        </div>,
      )
    }
  }

  return (
    <div className="tiles" ref={el} onScroll={(e) => setScroll((e.target as HTMLDivElement).scrollTop)}>
      <div style={{ height: rows * rowH + GAP, position: 'relative' }}>{tiles}</div>
    </div>
  )
}
