import type { App } from './data'

/*
  Query language (all terms AND-ed, case-insensitive):
    payments                free text over name, appset, project, cluster, namespace, repo, path, labels
    "exact phrase"          quoted text
    -legacy                 negation (works for every term)
    name:api  appset:x  project:x  cluster:x  ns:x  repo:x  rev:main  ctx:prod
    health:degraded  sync:outofsync  label:team=core  label:team
    is:error  is:warning  is:problem  is:auto  is:manual  is:running  is:deleting
*/

type Term = { neg: boolean; field: string; value: string; exact: boolean }

const aliases: Record<string, string> = {
  as: 'appset', set: 'appset', applicationset: 'appset', proj: 'project', p: 'project',
  c: 'cluster', dest: 'cluster', namespace: 'ns', n: 'name', h: 'health', s: 'sync', l: 'label', context: 'ctx',
}

export function parseQuery(q: string): Term[] {
  const terms: Term[] = []
  const re = /(-?)(?:([a-zA-Z]+):)?(?:"([^"]*)"|(\S+))/g
  let m: RegExpExecArray | null
  while ((m = re.exec(q))) {
    const field = (m[2] ?? '').toLowerCase()
    const value = (m[3] ?? m[4] ?? '').toLowerCase()
    if (!value && !field) continue
    terms.push({ neg: m[1] === '-', field: aliases[field] ?? field, value, exact: m[3] !== undefined })
  }
  return terms
}

function test(a: App, t: Term, ctxNames: Map<string, string>): boolean {
  const v = t.value
  const has = (s: string) => (t.exact ? s.toLowerCase() === v : s.toLowerCase().includes(v))
  switch (t.field) {
    case '':
      return a.hay.includes(v)
    case 'name':
      return has(a.name)
    case 'appset':
      return (v === '' || v === 'none') && !t.exact ? !a.appSet : has(a.appSet)
    case 'project':
      return has(a.project)
    case 'cluster':
      return has(a.cluster) || has(a.clusterServer)
    case 'ns':
      return has(a.destNamespace)
    case 'repo':
      return a.repo.toLowerCase().includes(v) || a.path.toLowerCase().includes(v)
    case 'rev':
      return a.targetRev.toLowerCase().includes(v) || a.syncRev.toLowerCase().includes(v)
    case 'health':
      return a.health.toLowerCase().startsWith(v)
    case 'sync':
      return a.sync.toLowerCase().startsWith(v)
    case 'ctx':
      return (ctxNames.get(a.ctx) ?? '').toLowerCase().includes(v)
    case 'label': {
      if (!a.labels) return false
      const [k, val] = v.split('=')
      for (const lk in a.labels) {
        if (lk.toLowerCase() !== k) continue
        if (val === undefined) return true
        if (a.labels[lk].toLowerCase().includes(val)) return true
      }
      return false
    }
    case 'is':
      switch (v) {
        case 'error': case 'errors': case 'failed': return a.severity === 2
        case 'warning': case 'warn': return a.severity === 1
        case 'problem': case 'problems': return a.severity > 0
        case 'ok': case 'healthy': return a.severity === 0
        case 'auto': case 'autosync': return a.autoSync
        case 'manual': return !a.autoSync
        case 'running': case 'syncing': return a.opPhase === 'Running'
        case 'deleting': return a.deleting
        case 'outofsync': return a.sync === 'OutOfSync'
      }
      return false
    default:
      // unknown field: treat "foo:bar" as free text
      return a.hay.includes(t.field + ':' + v)
  }
}

export function filterApps(apps: Iterable<App>, q: string, ctxNames: Map<string, string>, pred?: (a: App) => boolean): App[] {
  const terms = parseQuery(q)
  const out: App[] = []
  outer: for (const a of apps) {
    if (pred && !pred(a)) continue
    for (const t of terms) if (test(a, t, ctxNames) === t.neg) continue outer
    out.push(a)
  }
  return out
}

export type SortKey = 'severity' | 'name' | 'appset' | 'project' | 'cluster' | 'health' | 'sync' | 'synced'

const healthRank: Record<string, number> = { Degraded: 0, Missing: 1, Progressing: 2, Suspended: 3, Unknown: 4, Healthy: 5 }
const syncRank: Record<string, number> = { OutOfSync: 0, Unknown: 1, Synced: 2 }

export function sortApps(list: App[], key: SortKey, desc: boolean) {
  const cmp: Record<SortKey, (a: App, b: App) => number> = {
    severity: (a, b) => b.severity - a.severity || a.name.localeCompare(b.name),
    name: (a, b) => a.name.localeCompare(b.name),
    appset: (a, b) => a.appSet.localeCompare(b.appSet) || a.name.localeCompare(b.name),
    project: (a, b) => a.project.localeCompare(b.project) || a.name.localeCompare(b.name),
    cluster: (a, b) => a.cluster.localeCompare(b.cluster) || a.name.localeCompare(b.name),
    health: (a, b) => (healthRank[a.health] ?? 9) - (healthRank[b.health] ?? 9) || a.name.localeCompare(b.name),
    sync: (a, b) => (syncRank[a.sync] ?? 9) - (syncRank[b.sync] ?? 9) || a.name.localeCompare(b.name),
    synced: (a, b) => (b.opFinishedAt || '').localeCompare(a.opFinishedAt || '') || a.name.localeCompare(b.name),
  }
  const f = cmp[key]
  list.sort(desc ? (a, b) => f(b, a) : f)
  return list
}
