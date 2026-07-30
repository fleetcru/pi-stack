import { createAppStore } from "@pi-stack/webby-shared/state/app-store"

export type { ServerConnectionSettings } from "@pi-stack/webby-shared/state/app-store"

export const useAppStore = createAppStore("pi-webby-ui")
