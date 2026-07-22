import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router"

import "./index.css"
import App from "./App.tsx"
import { PiServerProvider } from "@/api/provider.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"

// Disable right-click context menu in the desktop app
if (window.__TAURI_INTERNALS__) {
  document.addEventListener("contextmenu", (e) => e.preventDefault())
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <PiServerProvider>
      <ThemeProvider defaultTheme="dark">
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </ThemeProvider>
    </PiServerProvider>
  </StrictMode>
)
