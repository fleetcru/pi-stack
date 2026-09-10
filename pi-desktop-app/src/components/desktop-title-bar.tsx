import { Maximize2, Minus, X } from "lucide-react"
import { getCurrentWindow } from "@tauri-apps/api/window"

export function DesktopTitleBar() {
  const appWindow = "__TAURI_INTERNALS__" in window ? getCurrentWindow() : undefined

  return (
    <header className="flex h-10 shrink-0 items-center border-b border-border/80 bg-background text-foreground">
      <div data-tauri-drag-region className="flex min-w-0 flex-1 items-center px-3">
        <span className="select-none text-xs font-semibold tracking-tight">Pi-Desktop</span>
      </div>
      <div className="flex h-full shrink-0 items-center">
        <button type="button" aria-label="Minimize window" className="flex h-full w-11 items-center justify-center text-white transition-colors hover:bg-muted" onClick={() => void appWindow?.minimize()}>
          <Minus className="size-4" />
        </button>
        <button type="button" aria-label="Maximize window" className="flex h-full w-11 items-center justify-center text-white transition-colors hover:bg-muted" onClick={() => void appWindow?.toggleMaximize()}>
          <Maximize2 className="size-3.5" />
        </button>
        <button type="button" aria-label="Close window" className="flex h-full w-11 items-center justify-center text-white transition-colors hover:bg-destructive hover:text-destructive-foreground" onClick={() => void appWindow?.close()}>
          <X className="size-4" />
        </button>
      </div>
    </header>
  )
}
