import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './index.css'

// Update page title from runtime env
const titleEl = document.querySelector('title')
if (titleEl && window.__env__?.VITE_APP_TITLE && !window.__env__.VITE_APP_TITLE.startsWith('$')) {
  titleEl.textContent = window.__env__.VITE_APP_TITLE
}

const root = document.getElementById('root')
if (!root) throw new Error('Root element not found')

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
