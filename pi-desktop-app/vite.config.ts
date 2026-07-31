import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      "@pi-stack/webby-shared": path.resolve(__dirname, "../pi-webby-shared/src"),
    },
    dedupe: ["react", "react-dom", "zustand", "@tanstack/react-query"],
  },
  server: {
    watch: {
      ignored: ["**/src-tauri/**"],
    },
  },
})
