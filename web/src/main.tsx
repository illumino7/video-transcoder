import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

// Application entrypoint. Bootstraps the React tree and injects it into the DOM
// under StrictMode to catch lifecycle and rendering anomalies during development.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
