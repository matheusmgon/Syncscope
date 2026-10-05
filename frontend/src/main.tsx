import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'

// one-time migration of UI preferences saved before the rename (ArgoDeck → Syncscope)
try {
  for (const k of Object.keys(localStorage)) {
    if (k.startsWith('argodeck.')) {
      const nk = 'syncscope.' + k.slice('argodeck.'.length)
      if (localStorage.getItem(nk) === null) localStorage.setItem(nk, localStorage.getItem(k) ?? '')
      localStorage.removeItem(k)
    }
  }
} catch { /* storage unavailable */ }

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
)
