import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { Toaster } from '@/components/ui/sonner'
import { ThemeProvider } from '@/hooks/use-theme'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="light">
      <App />
      <Toaster position="bottom-center" />
    </ThemeProvider>
  </StrictMode>,
)
