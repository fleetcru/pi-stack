import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router"
import { emit } from "@tauri-apps/api/event"

import "./index.css"
import App from "./App.tsx"
import { PiServerProvider } from "@/api/provider.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"

async function syncWebViewBackground() {
  if (!("__TAURI_INTERNALS__" in window)) return

  const { getCurrentWebview } = await import("@tauri-apps/api/webview")
  const applyBackground = () => {
    const isLight = document.documentElement.classList.contains("light")
    void getCurrentWebview().setBackgroundColor(isLight ? "#ffffff" : "#0a0a0b")
  }

  applyBackground()
  new MutationObserver(applyBackground).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  })
}

void syncWebViewBackground().catch(() => {
  // The browser development build has no Tauri WebView API.
})

// Disable right-click context menu in the desktop app
if ("__TAURI_INTERNALS__" in window) {
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

// Tell the native shell to reveal the window only after React has committed
// its first frame. In browser development, this is safely skipped.
if ("__TAURI_INTERNALS__" in window) {
  requestAnimationFrame(() => {
    void emit("pi-app-ready")
  })
}
