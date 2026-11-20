import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { installFetchInterceptor } from './lib/fetch-interceptor'

// Every API call carries the CSRF header and reports 401s to the sign-in gate.
installFetchInterceptor()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
