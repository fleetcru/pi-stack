import { useState } from "react"

import { QuickSessionComposer } from "@/components/quick-session-composer"

/** Shared empty-workspace create surface + expand token for sidebar "+" on empty. */
export function useWorkspaceCreateRequest(selectedSessionId: string | undefined, openSession: (id?: string) => void, setCreateSessionOpen: (open: boolean) => void) {
  const [composerExpandToken, setComposerExpandToken] = useState(0)
  const requestCreate = () => {
    if (selectedSessionId) setCreateSessionOpen(true)
    else {
      openSession(undefined)
      setComposerExpandToken((token) => token + 1)
    }
  }
  return { composerExpandToken, requestCreate }
}

export function EmptyWorkspace({ onCreated, expandMoreOptionsToken = 0 }: { onCreated: (sessionId: string) => void; expandMoreOptionsToken?: number }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-4 py-8 sm:px-8">
      <QuickSessionComposer onCreated={onCreated} expandMoreOptionsToken={expandMoreOptionsToken} />
    </div>
  )
}
