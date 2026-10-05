// Per-user conveniences kept in localStorage: favorites, saved searches and
// recently opened items. Small external store so components re-render on change.
import { useSyncExternalStore } from 'react'

export type SavedSearch = { id: string; name: string; scope: string; query: string }
export type Recent = { key: string; label: string; scope: string; at: number }

type State = { favorites: Set<string>; saved: SavedSearch[]; recent: Recent[] }

function read<T>(k: string, d: T): T {
  try {
    const v = localStorage.getItem('syncscope.' + k)
    return v ? (JSON.parse(v) as T) : d
  } catch {
    return d
  }
}
function write(k: string, v: unknown) {
  try {
    localStorage.setItem('syncscope.' + k, JSON.stringify(v))
  } catch { /* storage unavailable */ }
}

let state: State = {
  favorites: new Set(read<string[]>('favorites', [])),
  saved: read<SavedSearch[]>('savedSearches', []),
  recent: read<Recent[]>('recent', []),
}
const listeners = new Set<() => void>()
function set(next: Partial<State>) {
  state = { ...state, ...next }
  if (next.favorites) write('favorites', [...next.favorites])
  if (next.saved) write('savedSearches', next.saved)
  if (next.recent) write('recent', next.recent)
  listeners.forEach((l) => l())
}

export function usePrefs() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}

export const isFavorite = (key: string) => state.favorites.has(key)

export function toggleFavorite(key: string) {
  const f = new Set(state.favorites)
  f.has(key) ? f.delete(key) : f.add(key)
  set({ favorites: f })
}

export function saveSearch(name: string, scope: string, query: string) {
  set({ saved: [...state.saved.filter((s) => !(s.name === name && s.scope === scope)), { id: String(Date.now()), name, scope, query }] })
}

export function deleteSearch(id: string) {
  set({ saved: state.saved.filter((s) => s.id !== id) })
}

export function pushRecent(key: string, label: string, scope: string) {
  set({ recent: [{ key, label, scope, at: Date.now() }, ...state.recent.filter((r) => r.key !== key)].slice(0, 20) })
}
