import path from "path"
import { defineConfig } from "vitest/config"

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
    alias: {
      "@pi-stack/webby-shared": path.resolve(__dirname, "../pi-webby-shared/src"),
    },
    dedupe: ["react", "react-dom", "zustand", "@tanstack/react-query"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/init-shared.ts"],
  },
})
