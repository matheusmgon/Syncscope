import { useMemo, useState } from 'react'
import * as API from '../../../wailsjs/go/main/App'
import { filterK, getK, useKData, type KObj } from '../../kdata'
import { ConfirmButton, KPage, PhaseIcon, PodsAndLogs, Problem1, useAction, VTable, type Column } from './Common'
import { NoKube } from './Workflows'

type Notify = (m: string, ok: boolean) => void
type Sub = 'flow' | 'sources' | 'sensors' | 'bus'
type Src = { type: string; events: string[] }
type Dep = { name: string; eventSourceName: string; eventName: string }
type Trig = { name: string; type: string; conditions?: string }

export function EventsView({ query, ctxFilter, onOpen }: { query: string; ctxFilter: Set<string>; onOpen: (key: string) => void }) {
  const data = useKData()
  const [sub, setSub] = useState<Sub>('flow')
  const all = useMemo(
    () => [...data.objs.values()].filter((o) => ['EventSource', 'Sensor', 'EventBus'].includes(o.kind) && (!ctxFilter.size || ctxFilter.has(o.ctx))),
    [data.version, ctxFilter], // eslint-disable-line react-hooks/exhaustive-deps
  )
  const by = (k: string) => filterK(all.filter((o) => o.kind === k), query).sort((a, b) => b.severity - a.severity || a.name.localeCompare(b.name))
  const sources = by('EventSource'), sensors = by('Sensor'), buses = by('EventBus')
  const failing = all.filter((o) => o.severity === 2).length

  const base: Column<KObj>[] = [
    { header: '', width: '26px', render: (o) => <PhaseIcon kind={o.kind} phase={o.phase} /> },
    { header: 'Name', width: 'minmax(180px,1.5fr)', render: (o) => <b>{o.name}</b> },
    { header: 'Namespace', width: 'minmax(110px,1fr)', render: (o) => <span className="muted">{o.namespace}<span className="sub">{o.ctx}</span></span> },
  ]
  const srcCols: Column<KObj>[] = [...base,
    { header: 'Sources', width: 'minmax(200px,2fr)', render: (o) => <span>{((o.fields.sources as Src[]) ?? []).map((s) => `${s.type} (${s.events.join(', ')})`).join(' · ')}</span> },
    { header: 'Problem', width: 'minmax(220px,2.5fr)', render: (o) => <Problem1 o={o} /> }]
  const senCols: Column<KObj>[] = [...base,
    { header: 'Depends on', width: 'minmax(180px,1.6fr)', render: (o) => <span className="mono">{((o.fields.dependencies as Dep[]) ?? []).map((d) => `${d.eventSourceName}:${d.eventName}`).join(', ')}</span> },
    { header: 'Triggers', width: 'minmax(180px,1.6fr)', render: (o) => <span>{((o.fields.triggers as Trig[]) ?? []).map((t) => `${t.name} (${t.type})`).join(', ')}</span> },
    { header: 'Problem', width: 'minmax(200px,2fr)', render: (o) => <Problem1 o={o} /> }]
  const busCols: Column<KObj>[] = [...base,
    { header: 'Type', width: '120px', render: (o) => <span>{String(o.fields.type ?? '')}</span> },
    { header: 'Problem', width: 'minmax(220px,3fr)', render: (o) => <Problem1 o={o} /> }]

  return (
    <>
      <div className="filterbar">
        <div className="seg small">
          <button className={sub === 'flow' ? 'on' : ''} onClick={() => setSub('flow')}>Event flow</button>
          <button className={sub === 'sources' ? 'on' : ''} onClick={() => setSub('sources')}>EventSources ({sources.length})</button>
          <button className={sub === 'sensors' ? 'on' : ''} onClick={() => setSub('sensors')}>Sensors ({sensors.length})</button>
          <button className={sub === 'bus' ? 'on' : ''} onClick={() => setSub('bus')}>EventBus ({buses.length})</button>
        </div>
        <span className="spacer" />
        {failing > 0 && <span className="badge error">{failing} failing</span>}
      </div>
      {sub === 'flow' && <Flow sources={sources} sensors={sensors} onOpen={onOpen} />}
      {sub === 'sources' && <VTable rows={sources} columns={srcCols} rowKey={(o) => o.key} onOpen={(o) => onOpen(o.key)} rowClass={(o) => (o.severity === 2 ? 'sev-2' : '')} empty={<NoKube what="event sources" />} />}
      {sub === 'sensors' && <VTable rows={sensors} columns={senCols} rowKey={(o) => o.key} onOpen={(o) => onOpen(o.key)} rowClass={(o) => (o.severity === 2 ? 'sev-2' : '')} empty={<NoKube what="sensors" />} />}
      {sub === 'bus' && <VTable rows={buses} columns={busCols} rowKey={(o) => o.key} onOpen={(o) => onOpen(o.key)} rowClass={(o) => (o.severity === 2 ? 'sev-2' : '')} empty={<NoKube what="event buses" />} />}
    </>
  )
}

// Event sources → sensors → triggers, like the Argo Workflows "Event Flow" page.
const FW = 230, FH = 44, GAP = 14, COLS = [0, 360, 720]

function Flow({ sources, sensors, onOpen }: { sources: KObj[]; sensors: KObj[]; onOpen: (k: string) => void }) {
  type N = { id: string; x: number; y: number; label: string; sub: string; obj?: KObj; bad?: boolean }
  const nodes: N[] = []
  const edges: { a: string; b: string; bad: boolean }[] = []
  let y0 = 0
  // one node per (event source, event name)
  const srcNode = new Map<string, string>()
  for (const s of sources) {
    for (const src of (s.fields.sources as Src[]) ?? []) {
      for (const ev of src.events) {
        const id = `src|${s.ctx}|${s.namespace}|${s.name}|${ev}`
        srcNode.set(`${s.ctx}|${s.namespace}|${s.name}|${ev}`, id)
        nodes.push({ id, x: COLS[0], y: y0, label: `${s.name} · ${ev}`, sub: src.type, obj: s, bad: s.severity === 2 })
        y0 += FH + GAP
      }
    }
  }
  let y1 = 0, y2 = 0
  for (const s of sensors) {
    const deps = (s.fields.dependencies as Dep[]) ?? []
    const trigs = (s.fields.triggers as Trig[]) ?? []
    const id = `sen|${s.key}`
    const h = Math.max(1, trigs.length) * (FH + GAP)
    const ys = Math.max(y1, y2)
    nodes.push({ id, x: COLS[1], y: ys + (h - FH - GAP) / 2, label: s.name, sub: `${deps.length} dependencies`, obj: s, bad: s.severity === 2 })
    for (const d of deps) {
      const from = srcNode.get(`${s.ctx}|${s.namespace}|${d.eventSourceName}|${d.eventName}`)
      if (from) edges.push({ a: from, b: id, bad: s.severity === 2 || !!nodes.find((n) => n.id === from)?.bad })
    }
    trigs.forEach((t, i) => {
      const tid = `trg|${s.key}|${t.name}`
      nodes.push({ id: tid, x: COLS[2], y: ys + i * (FH + GAP), label: t.name, sub: t.type + (t.conditions ? ` · when ${t.conditions}` : ''), obj: s })
      edges.push({ a: id, b: tid, bad: s.severity === 2 })
    })
    y1 = y2 = ys + h
  }
  const pos = new Map(nodes.map((n) => [n.id, n]))
  const height = Math.max(y0, y1, y2) + 20
  if (!nodes.length) return <NoKube what="event sources or sensors" />
  return (
    <div className="rtree-canvas" style={{ flex: 1 }}>
      <div className="flow-heads"><span style={{ left: COLS[0] + 20 }}>Event sources</span><span style={{ left: COLS[1] + 20 }}>Sensors</span><span style={{ left: COLS[2] + 20 }}>Triggers</span></div>
      <div style={{ position: 'relative', width: COLS[2] + FW + 40, height, margin: '8px 20px 20px' }}>
        <svg width={COLS[2] + FW + 40} height={height} className="rtree-edges">
          {edges.map(({ a, b, bad }) => {
            const p = pos.get(a)!, q = pos.get(b)!
            const x1 = p.x + FW, y1 = p.y + FH / 2, x2 = q.x, y2 = q.y + FH / 2, mx = (x1 + x2) / 2
            return <path key={a + b} d={`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} className={bad ? 'bad' : ''} />
          })}
        </svg>
        {nodes.map((n) => (
          <div key={n.id} className={'rnode' + (n.bad ? ' degraded' : '') + (n.id.startsWith('trg') ? ' child' : '')}
            style={{ left: n.x, top: n.y, width: FW, height: FH, marginLeft: 0 }}
            onClick={() => n.obj && onOpen(n.obj.key)} title={n.obj?.problems?.[0]?.message}>
            <span className="kicon" style={{ width: 28, height: 28, fontSize: 8, background: n.id.startsWith('src') ? '#7b61ff' : n.id.startsWith('sen') ? '#00a2b3' : '#e8833a' }}>
              {n.id.startsWith('src') ? 'es' : n.id.startsWith('sen') ? 'sn' : 'trg'}
            </span>
            <div className="rnode-text">
              <div className="rnode-name">{n.label}</div>
              <div className={'rnode-kind' + (n.bad ? ' err' : '')}>{n.bad ? n.obj?.problems?.[0]?.message : n.sub}</div>
            </div>
            {n.obj && !n.id.startsWith('trg') && <PhaseIcon kind={n.obj.kind} phase={n.obj.phase} />}
          </div>
        ))}
      </div>
    </div>
  )
}

export function EventObjPage({ objKey, left, onClose, notify }: { objKey: string; left: number; onClose: () => void; notify: Notify }) {
  const data = useKData()
  const o = getK(objKey)
  const run = useAction(notify)
  void data.version
  if (!o) return null
  const restart = async () => {
    try {
      const n = await API.RestartPods(objKey)
      notify(`Restarted ${n} pod(s)`, true)
    } catch (e) {
      notify(`Restart failed: ${e}`, false)
    }
  }
  return (
    <KPage obj={o} left={left} onClose={onClose} icon={o.kind === 'EventSource' ? 'es' : o.kind === 'Sensor' ? 'sn' : 'bus'}
      subtitle={o.kind !== 'EventBus' ? <>event bus <b>{String(o.fields.eventBusName ?? 'default')}</b></> : <>{String(o.fields.type ?? '')}</>}
      actions={
        <>
          <ConfirmButton label="↻ Restart pods" confirm="Delete the pods so they restart?" onConfirm={restart} />
          <span className="spacer" />
          <ConfirmButton className="danger-outline" label="🗑 Delete" confirm={`Delete ${o.name}?`} onConfirm={() => run('Delete', () => API.KDelete(objKey)).then(onClose)} />
        </>
      }
      tabs={[
        {
          id: 'overview', label: 'Overview',
          render: () => (
            <>
              {o.kind === 'EventSource' && (
                <table className="mini-table"><thead><tr><th>Type</th><th>Events</th></tr></thead>
                  <tbody>{((o.fields.sources as Src[]) ?? []).map((s) => <tr key={s.type}><td><b>{s.type}</b></td><td className="mono">{s.events.join(', ')}</td></tr>)}</tbody>
                </table>
              )}
              {o.kind === 'Sensor' && (
                <>
                  <h4>Dependencies</h4>
                  <table className="mini-table"><thead><tr><th>Name</th><th>Event source</th><th>Event</th></tr></thead>
                    <tbody>{((o.fields.dependencies as Dep[]) ?? []).map((d) => <tr key={d.name}><td>{d.name}</td><td>{d.eventSourceName}</td><td className="mono">{d.eventName}</td></tr>)}</tbody>
                  </table>
                  <h4 style={{ marginTop: 16 }}>Triggers</h4>
                  <table className="mini-table"><thead><tr><th>Name</th><th>Type</th><th>Conditions</th></tr></thead>
                    <tbody>{((o.fields.triggers as Trig[]) ?? []).map((t) => <tr key={t.name}><td>{t.name}</td><td>{t.type}</td><td className="mono">{t.conditions || '—'}</td></tr>)}</tbody>
                  </table>
                </>
              )}
              {o.kind === 'EventBus' && <div className="kv"><div className="k">Type</div><div className="v">{String(o.fields.type)}</div></div>}
            </>
          ),
        },
        { id: 'pods', label: 'Pods & logs', flush: true, render: () => <PodsAndLogs objKey={objKey} /> },
      ]}
    />
  )
}
