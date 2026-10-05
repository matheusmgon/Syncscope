import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { argocd, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// Interactive shell in a pod through the Argo CD web terminal.
export function TerminalView({ appKey, node }: { appKey: string; node: store.TreeNode }) {
  const host = useRef<HTMLDivElement>(null)
  const [containers, setContainers] = useState<string[]>([])
  const [container, setContainer] = useState('')
  const [session, setSession] = useState(0) // bump to reconnect
  const [state, setState] = useState<{ s: 'idle' | 'connecting' | 'open' | 'closed' | 'error'; msg?: string }>({ s: 'idle' })

  useEffect(() => {
    API.Containers(appKey, argocd.ResourceAction.createFrom({ Group: node.group, Version: node.version, Kind: node.kind, Namespace: node.namespace, Name: node.name }))
      .then((cs) => {
        setContainers(cs ?? [])
        setContainer((cs ?? []).find((c) => !/^(istio-proxy|linkerd-proxy|envoy|vault-agent)$/.test(c)) ?? cs?.[0] ?? '')
      })
      .catch(() => setContainers([]))
  }, [appKey, node])

  useEffect(() => {
    if (!host.current || !container) return
    const dark = document.documentElement.dataset.theme === 'dark'
    const term = new Terminal({
      fontFamily: 'SF Mono, Menlo, Consolas, monospace', fontSize: 12, cursorBlink: true, scrollback: 5000,
      theme: { background: '#0b141b', foreground: '#d5dee4', cursor: '#00a2b3', selectionBackground: dark ? '#11414a' : '#1d4152' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host.current)
    fit.fit()
    let id = ''
    let dead = false
    setState({ s: 'connecting' })
    term.writeln(`\x1b[90mConnecting to ${node.namespace}/${node.name} (${container})…\x1b[0m`)
    // chunks are numbered by the backend; apply them strictly in order
    let next = 1
    const pending = new Map<number, { data?: string; closed?: boolean; error?: string }>()
    const apply = (p: { data?: string; closed?: boolean; error?: string }) => {
      if (p.data) term.write(p.data)
      if (p.closed) {
        setState(p.error ? { s: 'error', msg: p.error } : { s: 'closed' })
        term.writeln('\r\n\x1b[90m[session closed]\x1b[0m')
      }
    }
    const off = EventsOn('term', (p: { id: string; seq?: number; data?: string; closed?: boolean; error?: string }) => {
      if (p.id !== id) return
      if (!p.seq) return apply(p)
      pending.set(p.seq, p)
      while (pending.has(next)) {
        apply(pending.get(next)!)
        pending.delete(next)
        next++
      }
    })
    API.StartTerminal(appKey, store.TerminalRequest.createFrom({ namespace: node.namespace, pod: node.name, container }))
      .then((x) => {
        if (dead) return API.TermClose(x)
        id = x
        setState({ s: 'open' })
        API.TermResize(id, term.rows, term.cols)
        term.focus()
      })
      .catch((e) => {
        setState({ s: 'error', msg: String(e) })
        term.writeln(`\r\n\x1b[31m${String(e)}\x1b[0m`)
      })
    const dataSub = term.onData((d) => id && API.TermInput(id, d))
    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
        if (id) API.TermResize(id, term.rows, term.cols)
      } catch { /* not visible */ }
    })
    ro.observe(host.current)
    return () => {
      dead = true
      off()
      dataSub.dispose()
      ro.disconnect()
      if (id) API.TermClose(id)
      term.dispose()
    }
  }, [appKey, node, container, session])

  return (
    <div className="logs">
      <div className="logs-bar">
        {containers.length > 0 && (
          <select className="inline" value={container} onChange={(e) => setContainer(e.target.value)} title="Container">
            {containers.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        )}
        <span className="muted-sm">
          {state.s === 'connecting' ? 'connecting…' : state.s === 'open' ? '● connected' : state.s === 'closed' ? 'closed' : state.s === 'error' ? 'error' : ''}
        </span>
        <span className="spacer" />
        {(state.s === 'closed' || state.s === 'error') && <button className="btn sm" onClick={() => setSession((n) => n + 1)}>Reconnect</button>}
      </div>
      {state.s === 'error' && state.msg && <div className="alert" style={{ margin: '0 12px 8px' }}>{state.msg}</div>}
      <div className="term-host" ref={host} />
    </div>
  )
}
