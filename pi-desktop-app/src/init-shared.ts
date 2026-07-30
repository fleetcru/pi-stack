import { setAppStoreHook } from "@pi-stack/webby-shared/api/hooks"
import { useAppStore } from "@/state/app-store"

// Inject the app-specific store hook into the shared hooks module.
// This must run before any shared hooks are called.
setAppStoreHook(useAppStore)
