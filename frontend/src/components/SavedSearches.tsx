import { useState } from 'react'
import { deleteSearch, saveSearch, usePrefs } from '../userprefs'

// "☆ Save" + "Saved ▾" next to a search box, per scope (tab or product).
export function SavedSearches({ scope, query, setQuery }: { scope: string; query: string; setQuery: (q: string) => void }) {
  const prefs = usePrefs()
  const [open, setOpen] = useState(false)
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState('')
  const mine = prefs.saved.filter((s) => s.scope === scope)
  return (
    <span style={{ position: 'relative', display: 'inline-flex', gap: 4 }}>
      {query.trim() && !naming && <button className="btn sm ghost" title="Save this search" onClick={() => { setName(''); setNaming(true) }}>☆ Save</button>}
      {naming && (
        <form onSubmit={(e) => { e.preventDefault(); if (name.trim()) { saveSearch(name.trim(), scope, query); setNaming(false) } }} style={{ display: 'inline-flex', gap: 4 }}>
          <input className="rtree-filter" style={{ width: 160 }} autoFocus placeholder="Name this search" value={name} onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && setNaming(false)} />
          <button className="btn sm primary" type="submit">Save</button>
        </form>
      )}
      {mine.length > 0 && <button className="btn sm ghost" onClick={() => setOpen(!open)}>Saved ({mine.length}) ▾</button>}
      {open && (
        <div className="menu" style={{ top: 30, minWidth: 260 }} onMouseLeave={() => setOpen(false)}>
          {mine.map((s) => (
            <div key={s.id} className="menu-item" style={{ display: 'flex', gap: 8, textTransform: 'none' }} onClick={() => { setQuery(s.query); setOpen(false) }}>
              <span style={{ flex: 1 }}><b>{s.name}</b><div className="muted-sm mono">{s.query}</div></span>
              <span className="muted-sm" onClick={(e) => { e.stopPropagation(); deleteSearch(s.id) }} title="Delete">✕</span>
            </div>
          ))}
        </div>
      )}
    </span>
  )
}
