import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { argocd, config, store } from '../wailsjs/go/models'
import * as API from '../wailsjs/go/main/App'
import { startData, useData, type App as AppRow } from './data'
import { filterApps, matchText, problemText, sortApps, type SortKey } from './search'
import { SIDEBAR_DEFAULT, Sidebar } from './components/Sidebar'
import { Settings } from './components/Settings'
import { AppSetPage } from './components/AppSetPage'
import { SyncPolicyDialog } from './components/Parameters'
import { AppTable, type GroupBy } from './components/AppTable'
import { AppTiles } from './components/AppTiles'
import { Detail } from './components/Detail'
import { ProblemsView } from './components/Problems'
import { ConfirmAction, ContextDialog, LoginDialog, Toasts, type ActionKind, type Toast } from './components/Dialogs'
import { HealthIcon, StatBar, SyncIcon } from './components/Status'

type Tab = 'apps' | 'problems' | 'appsets' | 'clusters'

const searchPlaceholder: Record<Tab, string> = {
  apps: 'Search applications…  e.g. payments  appset:api  cluster:prod  health:degraded  is:error  label:team=core  -legacy',
  problems: 'Search problems…  app name, message text, appset:x, cluster:x…',
  appsets: 'Search ApplicationSets…',
  clusters: 'Search clusters by name or server…',
}

const HEALTHS = ['Healthy', 'Progressing', 'Degraded', 'Suspended', 'Missing', 'Unknown']
const SYNCS = ['Synced', 'OutOfSync', 'Unknown']

function load<T>(k: string, d: T): T {
  try {
    const v = localStorage.getItem('syncscope.' + k)
    return v ? (JSON.parse(v) as T) : d
  } catch {
    return d
  }
}
function save(k: string, v: unknown) {
  try {
    localStorage.setItem('syncscope.' + k, JSON.stringify(v))
  } catch { /* ignore */ }
}

export default function App() {
  const data = useData()
  const [tab, setTab] = useState<Tab>('apps')
  // one search per tab: Applications, Problems, ApplicationSets and Clusters each keep their own query
  const [queries, setQueries] = useState<Record<Tab, string>>({ apps: '', problems: '', appsets: '', clusters: '' })
  const setQueryFor = (t: Tab, v: string | ((q: string) => string)) =>
    setQueries((qs) => ({ ...qs, [t]: typeof v === 'function' ? v(qs[t]) : v }))
  const query = queries[tab]
  const setQuery = (v: string | ((q: string) => string)) => setQueryFor(tab, v)
  const dq = useDeferredValue(queries.apps)
  const dProblems = useDeferredValue(queries.problems)
  const dSets = useDeferredValue(queries.appsets)
  const dClusters = useDeferredValue(queries.clusters)
  const [healthF, setHealthF] = useState<Set<string>>(new Set())
  const [syncF, setSyncF] = useState<Set<string>>(new Set())
  const [ctxF, setCtxF] = useState<Set<string>>(new Set())
  const [groupBy, setGroupBy] = useState<GroupBy>(() => load('groupBy', 'none'))
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>(() => load('sort', { key: 'severity', desc: false }))
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [focused, setFocused] = useState<string | null>(null)
  const [detail, setDetail] = useState<string | null>(null)
  const [appSetPage, setAppSetPage] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [bulkPolicy, setBulkPolicy] = useState(false)
  const [contexts, setContexts] = useState<config.Context[]>([])
  const [editCtx, setEditCtx] = useState<config.Context | 'new' | null>(null)
  const [loginCtx, setLoginCtx] = useState<config.Context | null>(null)
  const [confirm, setConfirm] = useState<{ kind: ActionKind; keys: string[] } | null>(null)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [theme, setTheme] = useState<string>(() => load('theme', window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'))
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() => load('sidebarCollapsed', false))
  const [view, setView] = useState<'list' | 'tiles'>(() => load('view', 'list'))
  const [sidebarWidth, setSidebarWidth] = useState<number>(() => load('sidebarWidth', SIDEBAR_DEFAULT))
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => { startData() }, [])
  useEffect(() => { document.documentElement.dataset.theme = theme; save('theme', theme) }, [theme])
  useEffect(() => save('groupBy', groupBy), [groupBy])
  useEffect(() => save('sort', sort), [sort])
  useEffect(() => save('sidebarCollapsed', sidebarCollapsed), [sidebarCollapsed])
  useEffect(() => save('sidebarWidth', sidebarWidth), [sidebarWidth])
  useEffect(() => save('view', view), [view])

  const refreshContexts = useCallback(() => API.Contexts().then((c) => setContexts(c ?? [])), [])
  useEffect(() => { refreshContexts() }, [refreshContexts, data.statuses])

  const ctxNames = useMemo(() => new Map(data.statuses.map((s) => [s.id, s.name])), [data.statuses])
  const allApps = useMemo(() => [...data.apps.values()], [data.version]) // eslint-disable-line react-hooks/exhaustive-deps

  // Base set = context filter only; used for facet counts.
  const base = useMemo(() => (ctxF.size ? allApps.filter((a) => ctxF.has(a.ctx)) : allApps), [allApps, ctxF])
  const searched = useMemo(() => filterApps(base, dq, ctxNames), [base, dq, ctxNames])
  const filtered = useMemo(() => {
    const out = searched.filter((a) => (!healthF.size || healthF.has(a.health)) && (!syncF.size || syncF.has(a.sync)))
    return sortApps(out, sort.key, sort.desc)
  }, [searched, healthF, syncF, sort])

  const problemApps = useMemo(() => (dProblems ? filterApps(base, dProblems, ctxNames, undefined, problemText) : base), [base, dProblems, ctxNames])

  const facet = useMemo(() => {
    const h: Record<string, number> = {}, s: Record<string, number> = {}
    let err = 0
    for (const a of searched) {
      h[a.health] = (h[a.health] ?? 0) + 1
      s[a.sync] = (s[a.sync] ?? 0) + 1
    }
    for (const a of base) if (a.severity === 2) err++
    return { h, s, err }
  }, [searched, base])

  // Drop selections that no longer exist.
  useEffect(() => {
    if (!selected.size) return
    let changed = false
    const s = new Set<string>()
    for (const k of selected) data.apps.has(k) ? s.add(k) : (changed = true)
    if (changed) setSelected(s)
  }, [data.version]) // eslint-disable-line react-hooks/exhaustive-deps

  const notify = useCallback((msg: string, ok: boolean) => {
    const id = String(Math.random())
    setToasts((t) => [{ id, kind: 'msg' as const, msg, ok }, ...t].slice(0, 6))
    if (ok) setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000)
  }, [])

  const runAction = useCallback(async (kind: ActionKind, keys: string[], o?: argocd.SyncOptions, d?: store.DeleteOptions) => {
    setConfirm(null)
    let rep
    switch (kind) {
      case 'sync': rep = await API.Sync(keys, o ?? argocd.SyncOptions.createFrom({})); break
      case 'refresh': rep = await API.Refresh(keys, false); break
      case 'hard': rep = await API.Refresh(keys, true); break
      case 'restart': rep = await API.Restart(keys); break
      case 'terminate': rep = await API.Terminate(keys); break
      case 'delete': rep = await API.Delete(keys, d ?? store.DeleteOptions.createFrom({ cascade: true, policy: 'foreground' })); break
    }
    if (kind === 'delete') {
      const gone = new Set(rep.results.filter((r) => r.ok).map((r) => r.key))
      setDetail((cur) => (cur && gone.has(cur) ? null : cur))
      setSelected((sel) => new Set([...sel].filter((k) => !gone.has(k))))
    }
    const id = rep.id
    setToasts((t) => [{ id, kind: 'report' as const, report: rep }, ...t].slice(0, 6))
    if (!rep.failed) setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5000)
  }, [])

  // refresh is harmless: no confirmation. Everything else asks.
  const requestAction = useCallback((kind: ActionKind, keys: string[]) => {
    if (!keys.length) return
    if (kind === 'refresh' || kind === 'hard') runAction(kind, keys)
    else setConfirm({ kind, keys })
  }, [runAction])

  const addFilter = (term: string) => {
    setTab('apps')
    setQueryFor('apps', (q) => (q.includes(term) ? q : (q.trim() + ' ' + term).trim()))
  }

  // keyboard
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const inInput = (e.target as HTMLElement)?.closest('input, textarea, select')
      if ((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !inInput)) {
        e.preventDefault()
        searchRef.current?.focus()
        searchRef.current?.select()
        return
      }
      if (e.key === 'b' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setSidebarCollapsed((c) => !c)
        return
      }
      if (inInput) {
        if (e.key === 'Escape') (e.target as HTMLElement).blur()
        if (e.key === 'ArrowDown' && e.target === searchRef.current) {
          ;(e.target as HTMLElement).blur()
          setFocused(filtered[0]?.key ?? null)
        }
        return
      }
      if (detail || appSetPage || settingsOpen || confirm || editCtx || loginCtx || tab !== 'apps') return
      if (e.key === 'a' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setSelected(new Set(filtered.map((a) => a.key)))
      } else if (e.key === 'Escape') {
        setSelected(new Set())
      } else if (e.key === 'ArrowDown' || e.key === 'j' || e.key === 'ArrowUp' || e.key === 'k') {
        e.preventDefault()
        const i = filtered.findIndex((a) => a.key === focused)
        const d = e.key === 'ArrowDown' || e.key === 'j' ? 1 : -1
        const n = filtered[Math.max(0, Math.min(filtered.length - 1, i + d))]
        if (n) setFocused(n.key)
      } else if (e.key === 'Enter' && focused) {
        setDetail(focused)
      } else if (e.key === ' ' && focused) {
        e.preventDefault()
        const s = new Set(selected)
        s.has(focused) ? s.delete(focused) : s.add(focused)
        setSelected(s)
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [filtered, focused, selected, detail, appSetPage, settingsOpen, confirm, editCtx, loginCtx, tab])

  const toggleSet = (s: Set<string>, v: string, setter: (s: Set<string>) => void) => {
    const n = new Set(s)
    n.has(v) ? n.delete(v) : n.add(v)
    setter(n)
  }

  const errApps = useMemo(() => base.filter((a) => a.severity === 2), [base])
  const badSets = data.appsets.filter((s) => s.problems?.length && (!ctxF.size || ctxF.has(s.ctx)))
  const badClusters = data.clusters.filter((c) => c.state === 'Failed' && (!ctxF.size || ctxF.has(c.ctx)))
  const problemCount = errApps.length + badSets.length + badClusters.length
  const sel = [...selected]
  const selApps = sel.map((k) => data.apps.get(k)).filter(Boolean) as AppRow[]
  const ctxById = (id: string) => contexts.find((c) => c.id === id)

  const connected = data.statuses.some((s) => s.state === 'ok')
  const anyCtx = data.statuses.length > 0

  return (
    <div className="shell">
      <Sidebar
        statuses={data.statuses}
        apps={data.apps}
        version={data.version}
        selected={ctxF}
        onToggle={(id, multi) => {
          if (!id) return setCtxF(new Set())
          if (multi) toggleSet(ctxF, id, setCtxF)
          else setCtxF(ctxF.size === 1 && ctxF.has(id) ? new Set() : new Set([id]))
        }}
        onSettings={() => setSettingsOpen(true)}
        onEdit={(id) => setEditCtx(ctxById(id) ?? null)}
        onLogin={(id) => setLoginCtx(ctxById(id) ?? null)}
        onReconnect={(id) => API.Reconnect(id)}
        theme={theme}
        onTheme={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
        collapsed={sidebarCollapsed}
        onCollapse={setSidebarCollapsed}
        width={sidebarWidth}
        onResize={setSidebarWidth}
      />
      <div className="main">
        <div className="topbar">
          <div className="topbar-row">
            <div className="search">
              <span style={{ color: 'var(--fg-faint)' }}>⌕</span>
              <input
                ref={searchRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={searchPlaceholder[tab]}
                spellCheck={false}
              />
              {query && <button className="x" style={{ fontSize: 14 }} onClick={() => setQuery('')}>✕</button>}
              {tab === 'apps' && <span className="count">{filtered.length.toLocaleString('en-US')} / {base.length.toLocaleString('en-US')}</span>}
              <span className="kbd">⌘K</span>
            </div>
          </div>
          <div className="tabs">
            <div className={'tab' + (tab === 'apps' ? ' active' : '')} onClick={() => setTab('apps')}>
              Applications <span className="badge">{base.length.toLocaleString('en-US')}</span>
            </div>
            <div className={'tab' + (tab === 'problems' ? ' active' : '')} onClick={() => setTab('problems')}>
              Problems <span className={'badge' + (problemCount ? ' error' : '')}>{problemCount}</span>
            </div>
            <div className={'tab' + (tab === 'appsets' ? ' active' : '')} onClick={() => setTab('appsets')}>
              ApplicationSets <span className={'badge' + (badSets.length ? ' error' : '')}>{data.appsets.filter((s) => !ctxF.size || ctxF.has(s.ctx)).length}</span>
            </div>
            <div className={'tab' + (tab === 'clusters' ? ' active' : '')} onClick={() => setTab('clusters')}>
              Clusters <span className={'badge' + (badClusters.length ? ' error' : '')}>{data.clusters.filter((c) => !ctxF.size || ctxF.has(c.ctx)).length}</span>
            </div>
          </div>
        </div>

        {!anyCtx && (
          <div className="empty">
            <h3>Welcome to Syncscope</h3>
            <div className="muted-sm" style={{ marginBottom: 8 }}>Syncscope for Argo Apps</div>
            <p>Add an Argo CD instance or import the contexts you already use with the <code>argocd</code> CLI.</p>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'center' }}>
              <button className="btn primary" onClick={() => setEditCtx('new')}>＋ Add instance</button>
              <button className="btn" onClick={() => setSettingsOpen(true)}>Import from argocd CLI…</button>
            </div>
          </div>
        )}

        {anyCtx && tab === 'apps' && (
          <>
            {data.statuses.some((s) => s.cachedAt && s.state !== 'ok') && (
              <div className="banner info">
                <span>
                  ⟳ Showing cached data for {data.statuses.filter((s) => s.cachedAt && s.state !== 'ok').map((s) => s.name).join(', ')} while
                  {data.statuses.some((s) => s.cachedAt && s.state === 'auth') ? ' waiting for login' : ' refreshing from Argo CD'}…
                </span>
              </div>
            )}
            {problemCount > 0 && (
              <div className="banner" onClick={() => setTab('problems')}>
                <span>⚠ {errApps.length} {errApps.length === 1 ? 'failing application' : 'failing applications'}
                  {badSets.length ? ` · ${badSets.length} failing ApplicationSet(s)` : ''}
                  {badClusters.length ? ` · ${badClusters.length} unreachable cluster(s)` : ''}</span>
                <span className="examples">
                  {errApps.slice(0, 3).map((a) => `${a.name}: ${a.problems?.[0]?.message ?? ''}`).join('  ·  ')}
                </span>
                <span>View details →</span>
              </div>
            )}
            <div className="filterbar">
              {HEALTHS.map((h) => (
                <span key={h} className={'chip' + (healthF.has(h) ? ' on' : '')} onClick={() => toggleSet(healthF, h, setHealthF)}>
                  <HealthIcon status={h} />{h}<span className="n">{facet.h[h] ?? 0}</span>
                </span>
              ))}
              <span className="sep" />
              {SYNCS.map((s) => (
                <span key={s} className={'chip' + (syncF.has(s) ? ' on' : '')} onClick={() => toggleSet(syncF, s, setSyncF)}>
                  <SyncIcon status={s} />{s}<span className="n">{facet.s[s] ?? 0}</span>
                </span>
              ))}
              <span className="sep" />
              <span className={'chip' + (query.includes('is:error') ? ' on' : '')} onClick={() => setQuery((q) => (q.includes('is:error') ? q.replace(/\s*is:error/, '').trim() : (q + ' is:error').trim()))}>
                ⚠ Failing only<span className="n">{facet.err}</span>
              </span>
              <span className="spacer" />
              <div className="seg small" title="View">
                <button className={view === 'list' ? 'on' : ''} onClick={() => setView('list')}>☰ List</button>
                <button className={view === 'tiles' ? 'on' : ''} onClick={() => setView('tiles')}>▦ Tiles</button>
              </div>
              {view === 'list' && <>
              <span style={{ color: 'var(--fg-muted)', fontSize: 12 }}>Group by</span>
              <select className="inline" value={groupBy} onChange={(e) => setGroupBy(e.target.value as GroupBy)}>
                <option value="none">no grouping</option>
                <option value="appset">ApplicationSet</option>
                <option value="cluster">Cluster</option>
                <option value="project">Project</option>
                <option value="ctx">Argo CD instance</option>
              </select>
              </>}
            </div>
            {selected.size > 0 && (
              <div className="actionbar">
                <b>{selected.size} selected</b>
                <a onClick={() => setSelected(new Set())}>clear</a>
                <span className="spacer" />
                <button className="btn primary" onClick={() => requestAction('sync', sel)}>⟳ Sync</button>
                <button className="btn" onClick={() => requestAction('refresh', sel)}>Refresh</button>
                <button className="btn" onClick={() => requestAction('hard', sel)}>Hard refresh</button>
                <button className="btn" onClick={() => requestAction('restart', sel)}>↻ Restart</button>
                {selApps.some((a) => a.opPhase === 'Running') && <button className="btn danger" onClick={() => requestAction('terminate', selApps.filter((a) => a.opPhase === 'Running').map((a) => a.key))}>■ Terminate</button>}
                <button className="btn" onClick={() => setBulkPolicy(true)}>Sync policy…</button>
                <span className="sep" />
                <button className="btn danger-outline" onClick={() => requestAction('delete', sel)}>🗑 Delete</button>
              </div>
            )}
            {filtered.length === 0 ? (
              <div className="empty">
                {connected || data.apps.size ? (
                  <><h3>Nothing found</h3>No application matches the search and filters.</>
                ) : data.statuses.some((s) => s.state === 'auth') ? (
                  <><h3>Login required</h3>Click “Log in” on the instance in the sidebar.</>
                ) : (
                  <><h3>Connecting…</h3>Waiting for the instances to respond.</>
                )}
              </div>
            ) : view === 'tiles' ? (
              <AppTiles
                apps={filtered}
                ctxNames={ctxNames}
                selected={selected}
                setSelected={setSelected}
                focused={focused}
                onOpen={(k) => { setFocused(k); setDetail(k) }}
                onAction={requestAction}
                onAddFilter={addFilter}
              />
            ) : (
              <AppTable
                apps={filtered}
                groupBy={groupBy}
                ctxNames={ctxNames}
                selected={selected}
                setSelected={setSelected}
                focused={focused}
                onOpen={(k) => { setFocused(k); setDetail(k) }}
                onAddFilter={addFilter}
                sort={sort}
                setSort={setSort}
              />
            )}
          </>
        )}

        {anyCtx && tab === 'problems' && (
          <ProblemsView
            apps={problemApps}
            appsets={data.appsets.filter((s) => (!ctxF.size || ctxF.has(s.ctx)) && matchText(dProblems, s.name, ...(s.problems ?? []).map((p) => p.message)))}
            clusters={data.clusters.filter((c) => (!ctxF.size || ctxF.has(c.ctx)) && matchText(dProblems, c.name, c.server, c.message))}
            ctxNames={ctxNames}
            onOpen={(k) => setDetail(k)}
            onAction={requestAction}
            onFilterAppSet={(k) => setAppSetPage(k)}
          />
        )}

        {anyCtx && tab === 'appsets' && (
          <AppSetsView
            apps={base}
            query={dSets}
            statuses={data.statuses}
            appsets={data.appsets.filter((s) => !ctxF.size || ctxF.has(s.ctx))}
            ctxNames={ctxNames}
            onPick={(key) => setAppSetPage(key)}
          />
        )}

        {anyCtx && tab === 'clusters' && (
          <ClustersView
            apps={base}
            clusters={data.clusters.filter((c) => (!ctxF.size || ctxF.has(c.ctx)) && matchText(dClusters, c.name, c.server, c.version, c.state, ctxNames.get(c.ctx)))}
            ctxNames={ctxNames}
            onPick={(n) => addFilter(`cluster:"${n}"`)}
          />
        )}
      </div>

      {appSetPage && (
        <AppSetPage
          setKey={appSetPage}
          ctxName={ctxNames.get(appSetPage.split('|')[0]) ?? ''}
          left={sidebarCollapsed ? 76 : sidebarWidth}
          onClose={() => setAppSetPage(null)}
          onOpenApp={(k) => setDetail(k)}
          onShowInList={(n) => { setAppSetPage(null); setTab('apps'); setQueryFor('apps', `appset:"${n}"`) }}
          onAction={requestAction}
          notify={notify}
        />
      )}
      {detail && (
        <Detail
          appKey={detail}
          ctxName={ctxNames.get(detail.split('|')[0]) ?? ''}
          onClose={() => setDetail(null)}
          onAction={requestAction}
          notify={notify}
          left={sidebarCollapsed ? 76 : sidebarWidth}
          onOpenAppSet={(name) => {
            const ctx = detail.split('|')[0]
            const s = data.appsets.find((x) => x.ctx === ctx && x.name === name)
            setAppSetPage(s ? s.key : `${ctx}|/${name}`)
            setDetail(null)
          }}
        />
      )}
      {bulkPolicy && (
        <SyncPolicyDialog
          keys={sel}
          initial={{ automated: selApps.every((a) => a.autoSync), prune: false, selfHeal: false }}
          generatedCount={selApps.filter((a) => a.appSet).length}
          onClose={() => setBulkPolicy(false)}
          onApply={async (p) => {
            setBulkPolicy(false)
            const rep = await API.SetSyncPolicy(sel, p)
            setToasts((t) => [{ id: rep.id, kind: 'report' as const, report: rep }, ...t].slice(0, 6))
          }}
        />
      )}
      {settingsOpen && (
        <Settings
          statuses={data.statuses}
          contexts={contexts}
          theme={theme}
          setTheme={setTheme}
          view={view}
          setView={setView}
          onClose={() => setSettingsOpen(false)}
          onAdd={() => setEditCtx('new')}
          onEdit={(c) => setEditCtx(c)}
          onLogin={(c) => setLoginCtx(c)}
          notify={notify}
          refreshContexts={refreshContexts}
        />
      )}
      {editCtx && (
        <ContextDialog
          initial={editCtx === 'new' ? undefined : editCtx}
          onClose={() => { setEditCtx(null); refreshContexts() }}
          onSaved={async (c) => {
            setEditCtx(null)
            await refreshContexts()
            if (editCtx === 'new' && c.authType !== 'cli') setLoginCtx(c)
          }}
        />
      )}
      {loginCtx && (
        <LoginDialog
          ctx={loginCtx}
          onClose={() => setLoginCtx(null)}
          onDone={() => { setLoginCtx(null); notify(`Session updated for ${loginCtx.name}`, true) }}
        />
      )}
      {confirm && (
        <ConfirmAction
          kind={confirm.kind}
          names={confirm.keys.map((k) => {
            const a = data.apps.get(k)
            return a ? (ctxNames.size > 1 ? `${ctxNames.get(a.ctx)} / ${a.name}` : a.name) : k
          })}
          onClose={() => setConfirm(null)}
          onRun={(o, d) => runAction(confirm.kind, confirm.keys, o, d)}
        />
      )}
      <Toasts toasts={toasts} progress={data.progress} onClose={(id) => setToasts((t) => t.filter((x) => x.id !== id))} />
    </div>
  )
}

function AppSetsView({ apps, appsets, statuses, ctxNames, onPick, query }: {
  apps: AppRow[]; appsets: { key: string; ctx: string; name: string; namespace: string; problems?: { message: string }[] }[]
  statuses: { id: string; name: string; state: string; appSetsError?: string }[]
  ctxNames: Map<string, string>; onPick: (key: string) => void; query: string
}) {
  const stats = useMemo(() => {
    const m = new Map<string, { n: number; err: number; oos: number; h: Record<string, number> }>()
    for (const a of apps) {
      if (!a.appSet) continue
      const k = a.ctx + '|' + a.appSet
      let s = m.get(k)
      if (!s) m.set(k, (s = { n: 0, err: 0, oos: 0, h: {} }))
      s.n++
      if (a.severity === 2) s.err++
      if (a.sync === 'OutOfSync') s.oos++
      s.h[a.health] = (s.h[a.health] ?? 0) + 1
    }
    return m
  }, [apps])
  // Union of the ApplicationSets the API returned and the ones apps point to,
  // so the view still works without RBAC permission to list ApplicationSets.
  const rows = useMemo(() => {
    const byKey = new Map<string, { key: string; ctx: string; name: string; namespace: string; problems?: { message: string }[]; listed: boolean }>()
    for (const s of appsets) byKey.set(s.ctx + '|' + s.name, { ...s, listed: true })
    for (const k of stats.keys()) {
      if (byKey.has(k)) continue
      const [ctx, name] = [k.slice(0, k.indexOf('|')), k.slice(k.indexOf('|') + 1)]
      byKey.set(k, { key: ctx + '|/' + name, ctx, name, namespace: '', listed: false })
    }
    return [...byKey.values()]
      .filter((s) => matchText(query, s.name, s.namespace, ctxNames.get(s.ctx), ...(s.problems ?? []).map((p) => p.message)))
      .sort((a, b) => (b.problems?.length ?? 0) - (a.problems?.length ?? 0) || (stats.get(b.ctx + '|' + b.name)?.err ?? 0) - (stats.get(a.ctx + '|' + a.name)?.err ?? 0) || a.name.localeCompare(b.name))
  }, [appsets, stats, query, ctxNames])
  const noLinks = apps.length > 0 && stats.size === 0
  const errors = statuses.filter((s) => s.appSetsError)
  const unowned = apps.filter((a) => !a.appSet).length
  return (
    <div className="view">
      {errors.map((s) => (
        <div key={s.id} className="alert" style={{ marginBottom: 10 }}>
          <b>{s.name}</b>: could not list ApplicationSets — {s.appSetsError}. Apps are still grouped by their ApplicationSet owner below.
        </div>
      ))}
      {noLinks && (
        <div className="alert" style={{ marginBottom: 10 }}>
          None of the {apps.length} applications reports an ApplicationSet owner (metadata.ownerReferences). If your apps are generated by ApplicationSets, please report this along with your Argo CD version.
        </div>
      )}
      <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 10 }}>
        <span className="muted-sm">{rows.length} ApplicationSets · {unowned} apps not generated by an ApplicationSet</span>
      </div>
      <div className="card">
        <table className="grid">
          <thead><tr><th>ApplicationSet</th><th>Instance</th><th>Apps</th><th>Health</th><th>Failing</th><th>OutOfSync</th><th>ApplicationSet error</th></tr></thead>
          <tbody>
            {rows.map((s) => {
              const st = stats.get(s.ctx + '|' + s.name)
              return (
                <tr key={s.key} className="click" onClick={() => onPick(s.key)}>
                  <td><b>{s.name}</b>{!s.listed && <span className="muted-sm" title="Known only from the apps' owner references"> (from apps)</span>}</td>
                  <td>{ctxNames.get(s.ctx)}</td>
                  <td>{st?.n ?? 0}</td>
                  <td><StatBar total={st?.n ?? 0} counts={HEALTHS.map((h) => [h, st?.h[h] ?? 0])} /></td>
                  <td style={{ color: st?.err ? 'var(--error-fg)' : undefined, fontWeight: st?.err ? 600 : undefined }}>{st?.err ?? 0}</td>
                  <td>{st?.oos ?? 0}</td>
                  <td className="msg" style={{ color: 'var(--error-fg)' }}>{s.problems?.map((p) => p.message).join(' · ')}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {rows.length === 0 && <div className="empty">No ApplicationSets found.</div>}
      </div>
    </div>
  )
}

function ClustersView({ apps, clusters, ctxNames, onPick }: {
  apps: AppRow[]; clusters: { ctx: string; name: string; server: string; state: string; message?: string; version?: string }[]
  ctxNames: Map<string, string>; onPick: (name: string) => void
}) {
  const counts = useMemo(() => {
    const m = new Map<string, { n: number; err: number }>()
    for (const a of apps) {
      const k = a.ctx + '|' + a.clusterServer
      let c = m.get(k)
      if (!c) m.set(k, (c = { n: 0, err: 0 }))
      c.n++
      if (a.severity === 2) c.err++
    }
    return m
  }, [apps])
  return (
    <div className="view">
      <div className="card">
        <table className="grid">
          <thead><tr><th /><th>Cluster</th><th>Instance</th><th>Server</th><th>Version</th><th>Apps</th><th>Failing</th><th>Message</th></tr></thead>
          <tbody>
            {clusters.map((c) => {
              const n = counts.get(c.ctx + '|' + c.server)
              return (
                <tr key={c.ctx + c.server} className="click" onClick={() => onPick(c.name || c.server)}>
                  <td><span className={'dot ' + (c.state === 'Successful' ? 'ok' : c.state === 'Failed' ? 'error' : 'idle')} /></td>
                  <td><b>{c.name}</b></td>
                  <td>{ctxNames.get(c.ctx)}</td>
                  <td className="mono">{c.server}</td>
                  <td>{c.version}</td>
                  <td>{n?.n ?? 0}</td>
                  <td style={{ color: n?.err ? 'var(--error-fg)' : undefined }}>{n?.err ?? 0}</td>
                  <td className="msg" style={{ color: 'var(--error-fg)' }}>{c.state === 'Failed' ? c.message : ''}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {clusters.length === 0 && <div className="empty">No clusters visible (or no RBAC permission to list them).</div>}
      </div>
    </div>
  )
}
