// In-memory mirror of everything the backend streams. Kept outside React so
// that tens of thousands of apps can be patched cheaply; components subscribe
// through useSyncExternalStore and only re-render on a version bump.
import { useSyncExternalStore } from 'react'
import { store } from '../wailsjs/go/models'
import * as API from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

export type App = store.AppSummary & { hay: string }

type State = {
  version: number
  apps: Map<string, App>
  appsets: store.AppSetSummary[]
  clusters: store.ClusterSummary[]
  statuses: store.ContextStatus[]
  progress: Map<string, { id: string; action: string; done: number; total: number }>
}

const state: State = {
  version: 0,
  apps: new Map(),
  appsets: [],
  clusters: [],
  statuses: [],
  progress: new Map(),
}

const listeners = new Set<() => void>()
let snap = { ...state }
function bump() {
  state.version++
  snap = { ...state }
  listeners.forEach((l) => l())
}

function index(a: store.AppSummary): App {
  const parts = [a.name, a.appSet, a.project, a.cluster, a.clusterServer, a.destNamespace, a.repo, a.path, a.targetRev, a.appNamespace]
  if (a.labels) for (const k in a.labels) parts.push(k + '=' + a.labels[k])
  return Object.assign(a, { hay: parts.join('\u0001').toLowerCase() }) as App
}

export function useData() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => snap,
  )
}

export function getApp(key: string) {
  return state.apps.get(key)
}

let started = false
export async function startData() {
  if (started) return
  started = true

  EventsOn('apps:snapshot', (p: { ctx: string; apps: store.AppSummary[] | null }) => {
    const prefix = p.ctx + '|'
    for (const k of state.apps.keys()) if (k.startsWith(prefix)) state.apps.delete(k)
    for (const a of p.apps ?? []) state.apps.set(a.key, index(a))
    bump()
  })
  EventsOn('apps:delta', (p: { upserts: store.AppSummary[]; deletes: string[] }) => {
    for (const k of p.deletes ?? []) state.apps.delete(k)
    for (const a of p.upserts ?? []) state.apps.set(a.key, index(a))
    bump()
  })
  EventsOn('appsets', (s: store.AppSetSummary[]) => {
    state.appsets = s ?? []
    bump()
  })
  EventsOn('clusters', (s: store.ClusterSummary[]) => {
    state.clusters = s ?? []
    bump()
  })
  EventsOn('ctx:status', (s: store.ContextStatus[]) => {
    state.statuses = s ?? []
    bump()
  })
  EventsOn('action:progress', (p: { id: string; action: string; done: number; total: number }) => {
    state.progress = new Map(state.progress).set(p.id, p)
    bump()
  })
  EventsOn('action:done', (r: store.ActionReport) => {
    const m = new Map(state.progress)
    m.delete(r.id)
    state.progress = m
    bump()
  })

  const [apps, sets, clusters, statuses] = await Promise.all([API.Apps(), API.AppSets(), API.Clusters(), API.Statuses()])
  for (const a of apps ?? []) if (!state.apps.has(a.key)) state.apps.set(a.key, index(a))
  state.appsets = sets ?? []
  state.clusters = clusters ?? []
  state.statuses = statuses ?? []
  bump()
}
