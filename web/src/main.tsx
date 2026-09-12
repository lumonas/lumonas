import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource-variable/inter'
import '@/index.css'
import App from '@/App'

async function prepare() {
  if (import.meta.env.VITE_USE_MOCKS !== 'true') return
  const { worker } = await import('@/mocks/browser')
  await worker.start({ onUnhandledRequest: 'bypass', quiet: true })
}

prepare().then(() => {
  const container = document.getElementById('root')
  if (!container) throw new Error('root element not found')
  createRoot(container).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
  if (import.meta.env.PROD && 'serviceWorker' in navigator) {
    window.addEventListener('load', () => {
      navigator.serviceWorker.register('/sw.js').catch(() => void 0)
    })
  }
})
