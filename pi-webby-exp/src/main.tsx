import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router"

import "./index.css"
import App from "./App.tsx"
import { PiServerProvider } from "@/api/provider.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"

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
