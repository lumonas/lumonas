import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource-variable/inter'
import '@/index.css'
import App from '@/App'

async function prepare() {
  if (import.meta.env.VITE_USE_MOCKS === 'false') return
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
})
