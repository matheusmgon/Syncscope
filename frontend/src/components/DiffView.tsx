import { useMemo, useState } from 'react'
import { diffLines } from 'diff'
import { store } from '../../wailsjs/go/models'

// Live (cluster) vs desired (Git) manifests, unified diff with collapsed context.

type Row = { kind: 'ctx' | 'add' | 'del' | 'gap'; text: string; a?: number; b?: number }

const CONTEXT = 3

function unified(live: string, target: string): Row[] {
  const parts = diffLines(live, target)
  const rows: Row[] = []
  let a = 1, b = 1
  for (const p of parts) {
    const lines = p.value.replace(/\n$/, '').split('\n')
    for (const l of lines) {
      if (p.added) rows.push({ kind: 'add', text: l, b: b++ })
      else if (p.removed) rows.push({ kind: 'del', text: l, a: a++ })
      else rows.push({ kind: 'ctx', text: l, a: a++, b: b++ })
    }
  }
  // collapse unchanged runs longer than 2*CONTEXT
  const keep = rows.map(() => false)
  rows.forEach((r, i) => {
    if (r.kind === 'ctx') return
    for (let j = Math.max(0, i - CONTEXT); j <= Math.min(rows.length - 1, i + CONTEXT); j++) keep[j] = true
  })
  const out: Row[] = []
  let hidden = 0
  rows.forEach((r, i) => {
    if (keep[i]) {
      if (hidden) out.push({ kind: 'gap', text: `… ${hidden} unchanged lines` })
      hidden = 0
      out.push(r)
    } else hidden++
  })
  if (hidden && out.length) out.push({ kind: 'gap', text: `… ${hidden} unchanged lines` })
  return out
}

export function ResourceDiff({ item, full }: { item: store.DiffItem; full?: boolean }) {
  const rows = useMemo(() => {
    if (full) {
      return diffLines(item.live, item.target).flatMap((p) =>
        p.value.replace(/\n$/, '').split('\n').map((t): Row => ({ kind: p.added ? 'add' : p.removed ? 'del' : 'ctx', text: t })))
    }
    return unified(item.live, item.target)
  }, [item, full])
  if (!item.live && !item.target) return <div className="muted-sm">No manifest available.</div>
  if (!item.live) return <div className="help">Missing in the cluster — will be created on sync.<pre className="code-block">{item.target}</pre></div>
  if (!item.target) return <div className="help">Not in Git anymore — will be deleted when syncing with prune.<pre className="code-block">{item.live}</pre></div>
  if (!rows.some((r) => r.kind === 'add' || r.kind === 'del')) return <div className="muted-sm">No differences.</div>
  return (
    <div className="diff mono selectable">
      {rows.map((r, i) => (
        <div key={i} className={'dl k-' + r.kind}>
          <span className="ln">{r.a ?? ''}</span>
          <span className="ln">{r.b ?? ''}</span>
          <span className="sg">{r.kind === 'add' ? '+' : r.kind === 'del' ? '−' : ' '}</span>
          <span className="tx">{r.text}</span>
        </div>
      ))}
    </div>
  )
}

export function AppDiff({ items }: { items: store.DiffItem[] | null }) {
  const [onlyModified, setOnlyModified] = useState(true)
  const [full, setFull] = useState(false)
  const [open, setOpen] = useState<Set<string>>(new Set())
  if (!items) return <div className="help">Loading diff…</div>
  const modified = items.filter((i) => i.modified || !i.live || !i.target)
  const shown = onlyModified ? modified : items
  const id = (i: store.DiffItem) => `${i.group}/${i.kind}/${i.namespace}/${i.name}`
  return (
    <>
      <div style={{ display: 'flex', gap: 14, alignItems: 'center', marginBottom: 12 }}>
        <b>{modified.length} of {items.length} resources differ from Git</b>
        <label className="check"><input type="checkbox" checked={onlyModified} onChange={(e) => setOnlyModified(e.target.checked)} /> only changed</label>
        <label className="check"><input type="checkbox" checked={full} onChange={(e) => setFull(e.target.checked)} /> full manifests</label>
        <span className="spacer" />
        <span className="muted-sm"><span className="dl-key del">−</span> live (cluster) <span className="dl-key add">+</span> desired (Git)</span>
      </div>
      {shown.length === 0 && <div className="empty"><h3>In sync</h3>The cluster matches Git.</div>}
      {shown.map((i) => {
        const k = id(i)
        const isOpen = !open.has(k) // open by default; clicking collapses
        return (
          <div key={k} className="diff-card">
            <div className="diff-head" onClick={() => { const s = new Set(open); s.has(k) ? s.delete(k) : s.add(k); setOpen(s) }}>
              <span className="caret">{isOpen ? '▾' : '▸'}</span>
              <b>{i.kind}</b> <span className="mono">{i.namespace ? `${i.namespace}/` : ''}{i.name}</span>
              {!i.live && <span className="badge warning">missing</span>}
              {!i.target && <span className="badge warning">to prune</span>}
              {i.modified && i.live && i.target && <span className="badge warning">modified</span>}
              {i.hook && <span className="badge">hook</span>}
            </div>
            {isOpen && <ResourceDiff item={i} full={full} />}
          </div>
        )
      })}
    </>
  )
}
