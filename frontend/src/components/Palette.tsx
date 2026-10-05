import { useEffect, useMemo, useRef, useState } from 'react'
import { useData } from '../data'
import { useKData } from '../kdata'
import { usePrefs } from '../userprefs'
import { HealthIcon } from './Status'
import { phaseStatus } from './kube/Common'

// ⌘P command palette: jump to any app / ApplicationSet / workflow / rollout /
// event source, recent items and favorites first, plus app-wide commands.

export type PaletteItem = {
  id: string
  label: string
  hint?: string
  group: string
  status?: string
  run: () => void
}

type Props = {
  commands: PaletteItem[]
  openApp: (key: string) => void
  openAppSet: (key: string) => void
  openK: (key: string) => void
  onClose: () => void
}

function score(label: string, q: string): number {
  const l = label.toLowerCase()
  if (!q) return 1
  const i = l.indexOf(q)
  if (i === 0) return 100 - l.length / 100
  if (i > 0) return 60 - i / 10
  // subsequence match ("pyapi" ~ "payments-api")
  let j = 0
  for (const ch of l) if (ch === q[j]) j++
  return j === q.length ? 20 - l.length / 100 : -1
}

export function Palette({ commands, openApp, openAppSet, openK, onClose }: Props) {
  const data = useData()
  const k = useKData()
  const prefs = usePrefs()
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const list = useRef<HTMLDivElement>(null)
  const ctxNames = useMemo(() => new Map(data.statuses.map((s) => [s.id, s.name])), [data.statuses])

  const all = useMemo<PaletteItem[]>(() => {
    const items: PaletteItem[] = []
    for (const a of data.apps.values()) {
      items.push({ id: a.key, label: a.name, hint: `${ctxNames.get(a.ctx) ?? ''} · ${a.appSet || a.project}`, group: 'Applications', status: a.health, run: () => openApp(a.key) })
    }
    for (const s of data.appsets) items.push({ id: s.key, label: s.name, hint: `ApplicationSet · ${ctxNames.get(s.ctx) ?? ''}`, group: 'ApplicationSets', run: () => openAppSet(s.key) })
    for (const o of k.objs.values()) {
      items.push({ id: o.key, label: o.name, hint: `${o.kind} · ${o.ctx} · ${o.namespace}`, group: o.kind + 's', status: phaseStatus(o.kind, o.phase), run: () => openK(o.key) })
    }
    return items
  }, [data.version, k.version, ctxNames, openApp, openAppSet, openK]) // eslint-disable-line react-hooks/exhaustive-deps

  const results = useMemo(() => {
    const query = q.trim().toLowerCase()
    if (!query) {
      const byId = new Map(all.map((i) => [i.id, i]))
      const fav = [...prefs.favorites].map((id) => byId.get(id)).filter(Boolean).map((i) => ({ ...i!, group: '★ Favorites' }))
      const recent = prefs.recent.map((r) => byId.get(r.key)).filter(Boolean).map((i) => ({ ...i!, group: 'Recent' }))
      return [...recent.slice(0, 8), ...fav.slice(0, 12), ...commands]
    }
    const scored: [number, PaletteItem][] = []
    for (const c of commands) {
      const s = score(c.label, query)
      if (s > 0) scored.push([s + 5, c])
    }
    for (const i of all) {
      const s = score(i.label, query)
      if (s > 0) scored.push([s + (prefs.favorites.has(i.id) ? 10 : 0), i])
    }
    scored.sort((a, b) => b[0] - a[0])
    return scored.slice(0, 80).map(([, i]) => i)
  }, [q, all, commands, prefs])

  useEffect(() => setIdx(0), [q])
  useEffect(() => {
    list.current?.querySelector('.pal-item.on')?.scrollIntoView({ block: 'nearest' })
  }, [idx])

  const run = (i: PaletteItem | undefined) => {
    if (!i) return
    onClose()
    i.run()
  }

  let lastGroup = ''
  return (
    <div className="modal-backdrop palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="palette">
        <input
          autoFocus
          value={q}
          placeholder="Jump to an app, ApplicationSet, workflow, rollout… or type a command"
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onClose()
            else if (e.key === 'ArrowDown') { e.preventDefault(); setIdx((i) => Math.min(results.length - 1, i + 1)) }
            else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx((i) => Math.max(0, i - 1)) }
            else if (e.key === 'Enter') run(results[idx])
          }}
        />
        <div className="pal-list" ref={list}>
          {results.map((r, i) => {
            const head = r.group !== lastGroup ? r.group : null
            lastGroup = r.group
            return (
              <div key={r.group + r.id}>
                {head && <div className="pal-group">{head}</div>}
                <div className={'pal-item' + (i === idx ? ' on' : '')} onMouseEnter={() => setIdx(i)} onClick={() => run(r)}>
                  {r.status ? <HealthIcon status={r.status} /> : <span className="si">›</span>}
                  <span className="pal-label">{r.label}</span>
                  {r.hint && <span className="pal-hint">{r.hint}</span>}
                </div>
              </div>
            )
          })}
          {!results.length && <div className="pal-empty">No matches.</div>}
        </div>
        <div className="pal-foot">↑↓ navigate · ↵ open · esc close</div>
      </div>
    </div>
  )
}
