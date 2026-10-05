import { Fragment, useCallback, useEffect, useState } from 'react'
import { argocd, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { HealthIcon, SyncIcon, ago } from './Status'
import { LogViewer } from './LogViewer'
import { TerminalView } from './TerminalView'
import { YamlEditor } from './YamlEditor'
import { ResourceDiff } from './DiffView'
import { EventsView } from './EventsView'

export type PanelTab = 'details' | 'logs' | 'terminal' | 'manifest' | 'diff' | 'events'

export const kindAbbr: Record<string, string> = {
  Deployment: 'deploy', ReplicaSet: 'rs', Pod: 'pod', Service: 'svc', ConfigMap: 'cm', Secret: 'secret',
  Ingress: 'ing', StatefulSet: 'sts', DaemonSet: 'ds', Job: 'job', CronJob: 'cj', PersistentVolumeClaim: 'pvc',
  ServiceAccount: 'sa', Role: 'role', RoleBinding: 'rb', ClusterRole: 'c.role', ClusterRoleBinding: 'crb',
  HorizontalPodAutoscaler: 'hpa', Endpoints: 'ep', EndpointSlice: 'eps', Namespace: 'ns', NetworkPolicy: 'netpol',
  PodDisruptionBudget: 'pdb', Rollout: 'rollout', Application: 'app', ApplicationSet: 'appset',
  CustomResourceDefinition: 'crd', ServiceMonitor: 'sm', Certificate: 'cert', ExternalSecret: 'es',
}

const ref = (n: store.TreeNode) =>
  argocd.ResourceAction.createFrom({ Group: n.group, Version: n.version, Kind: n.kind, Namespace: n.namespace, Name: n.name })

type Props = {
  appKey: string
  node: store.TreeNode
  tab: PanelTab
  setTab: (t: PanelTab) => void
  selfHeal: boolean
  onClose: () => void
  notify: (m: string, ok: boolean) => void
}

export function ResourcePanel({ appKey, node, tab, setTab, selfHeal, onClose, notify }: Props) {
  const [narrowW, setNarrowW] = useState(() => Number(localStorage.getItem('syncscope.treeSideW')) || 380)
  const [wideW, setWideW] = useState(() => Number(localStorage.getItem('syncscope.treeLogsW')) || 760)
  const [actions, setActions] = useState<argocd.ActionDef[] | null>(null)
  const [menu, setMenu] = useState(false)
  const [confirmDel, setConfirmDel] = useState(false)
  const wide = tab !== 'details'
  const tabs: PanelTab[] = ['details', ...(node.hasLogs ? (['logs'] as PanelTab[]) : []), ...(node.kind === 'Pod' ? (['terminal'] as PanelTab[]) : []), 'manifest', ...(node.managed ? (['diff'] as PanelTab[]) : []), 'events']
  const current = tabs.includes(tab) ? tab : 'details'

  useEffect(() => {
    setActions(null)
    API.ResourceActions(appKey, ref(node)).then((a) => setActions(a ?? [])).catch(() => setActions([]))
  }, [appKey, node])

  useEffect(() => {
    try {
      localStorage.setItem('syncscope.treeSideW', String(narrowW))
      localStorage.setItem('syncscope.treeLogsW', String(wideW))
    } catch { /* ignore */ }
  }, [narrowW, wideW])

  const startResize = (e: React.MouseEvent) => {
    e.preventDefault()
    const x0 = e.clientX
    const w0 = wide ? wideW : narrowW
    document.body.classList.add('resizing')
    const move = (ev: MouseEvent) => {
      const w = Math.max(300, Math.min(window.innerWidth - 300, w0 + x0 - ev.clientX))
      wide ? setWideW(w) : setNarrowW(w)
    }
    const up = () => {
      document.body.classList.remove('resizing')
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', up)
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
  }

  const run = async (action: string) => {
    setMenu(false)
    try {
      await API.RunAction(appKey, ref(node), action)
      notify(`${action} sent to ${node.kind}/${node.name}`, true)
    } catch (e) {
      notify(`${action} failed on ${node.kind}/${node.name}: ${e}`, false)
    }
  }

  const syncThis = async () => {
    setMenu(false)
    const rep = await API.Sync([appKey], argocd.SyncOptions.createFrom({ resources: [{ group: node.group, kind: node.kind, name: node.name, namespace: node.namespace }] }))
    notify(rep.failed ? `Sync failed: ${rep.results[0]?.error}` : `Sync of ${node.kind}/${node.name} started`, !rep.failed)
  }

  const loadYaml = useCallback(() => API.ResourceYAML(appKey, ref(node)), [appKey, node])
  const saveYaml = useCallback((y: string) => API.PatchResourceYAML(appKey, ref(node), y), [appKey, node])
  const loadEvents = useCallback(() => API.ResourceEvents(appKey, ref(node), node.uid ?? ''), [appKey, node])

  return (
    <div className={'rtree-side selectable' + (wide ? ' logs-mode' : '')} style={{ width: wide ? wideW : narrowW }}>
      <div className="side-resizer" onMouseDown={startResize} title="Drag to resize" />
      <div className="rtree-side-head">
        <span className="kicon">{kindAbbr[node.kind] ?? node.kind.slice(0, 4).toLowerCase()}</span>
        <div style={{ minWidth: 0, flex: 1 }}>
          <div className="rnode-name" style={{ whiteSpace: 'normal', wordBreak: 'break-all' }}>{node.name}</div>
          <div className="rnode-kind">{node.group ? `${node.group}/` : ''}{node.version} {node.kind}{node.namespace ? ` · ${node.namespace}` : ''}</div>
        </div>
        <div style={{ position: 'relative' }}>
          <button className="btn sm" onClick={() => setMenu(!menu)}>Actions ▾</button>
          {menu && (
            <div className="menu" onMouseLeave={() => setMenu(false)}>
              {actions === null && <div className="menu-item muted-sm">loading…</div>}
              {(actions ?? []).map((a) => (
                <div key={a.name} className={'menu-item' + (a.disabled ? ' disabled' : '')} onClick={() => !a.disabled && run(a.name)}>
                  {a.displayName || a.name}
                </div>
              ))}
              {actions?.length === 0 && <div className="menu-item muted-sm">no actions for {node.kind}</div>}
              <div className="menu-sep" />
              {node.managed && <div className="menu-item" style={{ textTransform: 'none' }} onClick={syncThis}>⟳ Sync only this resource</div>}
              <div className="menu-item danger" onClick={() => { setMenu(false); setConfirmDel(true) }}>🗑 Delete resource…</div>
            </div>
          )}
        </div>
        <button className="x" onClick={onClose}>✕</button>
      </div>
      <div className="seg small" style={{ marginBottom: 12, flexWrap: 'wrap' }}>
        {tabs.map((t) => (
          <button key={t} className={current === t ? 'on' : ''} onClick={() => setTab(t)} style={{ textTransform: 'capitalize' }}>{t}</button>
        ))}
      </div>

      {current === 'details' && (
        <>
          {node.healthMsg && node.health !== 'Healthy' && (
            <div className="problem-box" style={{ margin: '0 0 12px' }}>
              <div className="problem-line error"><span className="tag error">{node.health}</span><div className="msg">{node.healthMsg}</div></div>
            </div>
          )}
          <div className="kv">
            <div className="k">Namespace</div><div className="v">{node.namespace || '—'}</div>
            <div className="k">Health</div><div className="v">{node.health ? <><HealthIcon status={node.health} /> {node.health}</> : '—'}</div>
            {node.managed && <><div className="k">Sync</div><div className="v"><SyncIcon status={node.sync || 'Unknown'} /> {node.sync}{node.prune ? ' (requires pruning)' : ''}{node.hook ? ' (hook)' : ''}</div></>}
            {node.createdAt && <><div className="k">Created</div><div className="v">{node.createdAt} ({ago(node.createdAt)} ago)</div></>}
            {Object.entries(node.info ?? {}).map(([k, v]) => (
              <Fragment key={k}><div className="k">{k}</div><div className="v">{v}</div></Fragment>
            ))}
            {(node.images?.length ?? 0) > 0 && <><div className="k">Images</div><div className="v mono">{node.images!.join('\n')}</div></>}
          </div>
          <div style={{ display: 'flex', gap: 6, marginTop: 14, flexWrap: 'wrap' }}>
            {node.restartable && <button className="btn" onClick={() => run('restart')}>↻ Restart</button>}
            {node.hasLogs && <button className="btn" onClick={() => setTab('logs')}>Logs</button>}
            {node.kind === 'Pod' && <button className="btn" onClick={() => setTab('terminal')}>&gt;_ Terminal</button>}
          </div>
        </>
      )}
      {current === 'logs' && <LogViewer key={node.id} appKey={appKey} node={node} />}
      {current === 'terminal' && <TerminalView key={node.id} appKey={appKey} node={node} />}
      {current === 'manifest' && (
        <div className="panel-scroll">
          <YamlEditor
            load={loadYaml}
            save={saveYaml}
            warning={
              <div className="alert" style={{ marginBottom: 8, background: 'var(--warning-bg)', color: 'var(--warning-fg)' }}>
                You are editing the <b>live</b> object in the cluster.{node.managed ? ' It is managed by Argo CD: the change makes the app OutOfSync' : ''}
                {node.managed && selfHeal ? ' and self-heal will revert it.' : node.managed ? ' and the next sync reverts it.' : '.'}
              </div>
            }
          />
        </div>
      )}
      {current === 'diff' && <div className="panel-scroll"><NodeDiff appKey={appKey} node={node} /></div>}
      {current === 'events' && <div className="panel-scroll"><EventsView load={loadEvents} compact /></div>}

      {confirmDel && <DeleteResourceDialog appKey={appKey} node={node} onClose={() => setConfirmDel(false)} notify={notify} />}
    </div>
  )
}

function NodeDiff({ appKey, node }: { appKey: string; node: store.TreeNode }) {
  const [item, setItem] = useState<store.DiffItem | null | undefined>(undefined)
  const [err, setErr] = useState('')
  useEffect(() => {
    API.Diff(appKey)
      .then((items) => setItem((items ?? []).find((i) => i.kind === node.kind && i.name === node.name && i.namespace === node.namespace && i.group === node.group) ?? null))
      .catch((e) => setErr(String(e)))
  }, [appKey, node])
  if (err) return <div className="alert">{err}</div>
  if (item === undefined) return <div className="help">Loading diff…</div>
  if (item === null) return <div className="muted-sm">This resource is not managed directly by the app (no desired state).</div>
  return <ResourceDiff item={item} />
}

function DeleteResourceDialog({ appKey, node, onClose, notify }: { appKey: string; node: store.TreeNode; onClose: () => void; notify: (m: string, ok: boolean) => void }) {
  const [mode, setMode] = useState<'foreground' | 'force' | 'orphan'>('foreground')
  const [typed, setTyped] = useState('')
  const go = async () => {
    try {
      await API.DeleteResource(appKey, ref(node), mode === 'force', mode === 'orphan')
      notify(`Deleted ${node.kind}/${node.name}`, true)
      onClose()
    } catch (e) {
      notify(`Delete failed: ${e}`, false)
    }
  }
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <header>Delete {node.kind} {node.name}</header>
        <div className="content">
          <div className="seg small">
            <button className={mode === 'foreground' ? 'on' : ''} onClick={() => setMode('foreground')}>Foreground</button>
            <button className={mode === 'force' ? 'on' : ''} onClick={() => setMode('force')}>Force</button>
            <button className={mode === 'orphan' ? 'on' : ''} onClick={() => setMode('orphan')}>Orphan (keep children)</button>
          </div>
          {node.managed && <div className="help">The resource is managed by Argo CD: it is re-created on the next sync unless removed from Git.</div>}
          {node.kind === 'Pod' && <div className="help">Deleting a pod owned by a ReplicaSet just makes it reschedule — a quick way to restart one pod.</div>}
          <div className="field">
            <label>Type <b className="mono">{node.name}</b> to confirm</label>
            <input type="text" value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
          </div>
        </div>
        <footer>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn danger" disabled={typed.trim() !== node.name} onClick={go}>Delete</button>
        </footer>
      </div>
    </div>
  )
}
