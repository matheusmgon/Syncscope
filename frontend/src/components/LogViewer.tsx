import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { argocd, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// Pod / workload logs for the resource tree side panel. Streams from the
// Argo CD logs endpoint (follow), keeps the last MAX_LINES in memory and
// renders the tail of whatever matches the filter.

const MAX_LINES = 20000
const RENDER_LINES = 2500
const TAILS = [100, 500, 1000, 5000]

type Line = { pod: string; time: string; content: string }

const errRe = /\b(error|err|fatal|panic|exception|failed|traceback)\b|"level":"(error|fatal)"/i
const warnRe = /\b(warn|warning)\b|"level":"warn/i

function podColor(pod: string) {
  let h = 0
  for (let i = 0; i < pod.length; i++) h = (h * 31 + pod.charCodeAt(i)) % 360
  return `hsl(${h} 60% 60%)`
}

export function LogViewer({ appKey, node }: { appKey: string; node: store.TreeNode }) {
  const [containers, setContainers] = useState<string[]>([])
  const [container, setContainer] = useState('')
  const [ready, setReady] = useState(false)
  const [tail, setTail] = useState(500)
  const [follow, setFollow] = useState(true)
  const [previous, setPrevious] = useState(false)
  const [wrap, setWrap] = useState(true)
  const [times, setTimes] = useState(false)
  const [filter, setFilter] = useState('')
  const [state, setState] = useState<{ status: 'loading' | 'streaming' | 'ended' | 'error'; error?: string }>({ status: 'loading' })
  const [version, setVersion] = useState(0)
  const lines = useRef<Line[]>([])
  const box = useRef<HTMLDivElement>(null)
  const stick = useRef(true)
  const [atBottom, setAtBottom] = useState(true)
  const frozen = useRef<{ total: number; list: Line[] } | null>(null)
  const aggregated = node.kind !== 'Pod'

  const ref = useMemo(
    () => argocd.ResourceAction.createFrom({ Group: node.group, Version: node.version, Kind: node.kind, Namespace: node.namespace, Name: node.name }),
    [node],
  )

  useEffect(() => {
    let cancel = false
    setContainers([])
    setContainer('')
    setReady(false)
    API.Containers(appKey, ref)
      .then((cs) => {
        if (cancel) return
        setContainers(cs ?? [])
        // skip common sidecars by default
        const main = (cs ?? []).find((c) => !/^(istio-proxy|linkerd-proxy|envoy|vault-agent|cloud-sql-proxy)$/.test(c)) ?? cs?.[0] ?? ''
        setContainer(main)
        setReady(true)
      })
      .catch(() => !cancel && (setContainer(''), setReady(true)))
    return () => { cancel = true }
  }, [appKey, ref])

  useEffect(() => {
    if (!ready) return
    let id = ''
    let dead = false
    lines.current = []
    stick.current = true
    frozen.current = null
    setAtBottom(true)
    setVersion((v) => v + 1)
    setState({ status: 'loading' })
    const off = EventsOn('logs', (p: { id: string; lines?: Line[]; done?: boolean; error?: string }) => {
      if (p.id !== id) return
      if (p.lines?.length) {
        const all = lines.current
        all.push(...p.lines)
        if (all.length > MAX_LINES) all.splice(0, all.length - MAX_LINES)
        setVersion((v) => v + 1)
        setState((s) => (s.status === 'loading' ? { status: 'streaming' } : s))
      }
      if (p.done) setState(p.error ? { status: 'error', error: p.error } : { status: 'ended' })
    })
    API.StartLogs(appKey, store.LogRequest.createFrom({
      group: node.group, version: node.version, kind: node.kind, namespace: node.namespace, name: node.name,
      container, tailLines: tail, follow, previous,
    }))
      .then((x) => {
        id = x
        if (dead) API.StopLogs(x)
      })
      .catch((e) => setState({ status: 'error', error: String(e) }))
    return () => {
      dead = true
      off()
      if (id) API.StopLogs(id)
    }
  }, [appKey, node, container, tail, follow, previous, ready])

  // While the user is scrolled up, the view is frozen so new lines never move
  // what they are reading; "↓ Latest" resumes.
  const live = useMemo(() => {
    const q = filter.trim().toLowerCase()
    const src = q ? lines.current.filter((l) => l.content.toLowerCase().includes(q) || l.pod.toLowerCase().includes(q)) : lines.current
    return { total: src.length, list: src.slice(-RENDER_LINES) }
  }, [version, filter]) // eslint-disable-line react-hooks/exhaustive-deps
  if (atBottom || !frozen.current) frozen.current = live
  const shown = atBottom ? live : frozen.current
  const pending = live.total - shown.total

  useLayoutEffect(() => {
    if (stick.current && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [shown])

  const copy = () => navigator.clipboard?.writeText(shown.list.map((l) => (aggregated ? `[${l.pod}] ` : '') + l.content).join('\n'))

  return (
    <div className="logs">
      <div className="logs-bar">
        {containers.length > 0 && (
          <select className="inline" value={container} onChange={(e) => setContainer(e.target.value)} title="Container">
            {containers.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        )}
        <select className="inline" value={tail} onChange={(e) => setTail(Number(e.target.value))} title="Tail lines">
          {TAILS.map((t) => <option key={t} value={t}>last {t}</option>)}
        </select>
        <label className="check"><input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} /> follow</label>
        <label className="check" title="Logs of the previous (crashed) container"><input type="checkbox" checked={previous} onChange={(e) => setPrevious(e.target.checked)} /> previous</label>
        <label className="check"><input type="checkbox" checked={wrap} onChange={(e) => setWrap(e.target.checked)} /> wrap</label>
        <label className="check"><input type="checkbox" checked={times} onChange={(e) => setTimes(e.target.checked)} /> time</label>
      </div>
      <div className="logs-bar">
        <input className="rtree-filter" style={{ flex: 1 }} placeholder="Filter lines…" value={filter} onChange={(e) => setFilter(e.target.value)} spellCheck={false} />
        <span className="muted-sm">
          {state.status === 'loading' ? 'loading…' : state.status === 'streaming' ? (follow ? '● live' : 'loading…') : state.status === 'ended' ? 'end' : 'error'}
          {' · '}{shown.total.toLocaleString('en-US')} lines
        </span>
        <button className="btn sm" onClick={copy} title="Copy visible lines">Copy</button>
      </div>
      {aggregated && <div className="muted-sm" style={{ padding: '0 12px 6px' }}>All pods of {node.kind} {node.name}</div>}
      {state.status === 'error' && <div className="alert" style={{ margin: '0 12px 8px' }}>{state.error}</div>}
      <div
        className={'logs-body mono' + (wrap ? ' wrap' : '')}
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget
          const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30
          stick.current = bottom
          if (bottom !== atBottom) setAtBottom(bottom)
        }}
      >
        {shown.total > shown.list.length && <div className="muted-sm">… {shown.total - shown.list.length} earlier lines hidden (use the filter)</div>}
        {shown.list.map((l, i) => (
          <div key={i} className={'logline' + (errRe.test(l.content) ? ' err' : warnRe.test(l.content) ? ' warn' : '')}>
            {times && <span className="lt">{l.time.replace('T', ' ').slice(0, 23)} </span>}
            {aggregated && <span className="lp" style={{ color: podColor(l.pod) }}>{l.pod.slice(-11)} </span>}
            {l.content}
          </div>
        ))}
        {state.status !== 'loading' && shown.total === 0 && <div className="muted-sm">No log lines{filter ? ' match the filter' : ''}.</div>}
      </div>
      {!atBottom && (
        <button className="btn sm logs-bottom" onClick={() => { stick.current = true; setAtBottom(true); requestAnimationFrame(() => box.current && (box.current.scrollTop = box.current.scrollHeight)) }}>
          ↓ Latest{pending > 0 ? ` (${pending} new)` : ''}
        </button>
      )}
    </div>
  )
}
