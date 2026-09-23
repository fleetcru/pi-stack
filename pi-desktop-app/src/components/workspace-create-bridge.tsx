import { useState } from "react"

import { QuickSessionComposer } from "@/components/quick-session-composer"

/**
 * Sidebar/header "+" entry:
 * - empty workspace → focus the center composer
 * - session selected → open NewSessionDialog (composer-only modal)
 */
export function useWorkspaceCreateRequest(
  selectedSessionId: string | undefined,
  openSession: (id?: string) => void,
  setNewSessionOpen: (open: boolean) => void,
) {
  const [composerFocusToken, setComposerFocusToken] = useState(0)
  const requestCreate = () => {
    if (selectedSessionId) setNewSessionOpen(true)
    else {
      openSession(undefined)
      setComposerFocusToken((token) => token + 1)
    }
  }
  return { composerFocusToken, requestCreate }
}

export function EmptyWorkspace({
  onCreated,
  focusToken = 0,
}: {
  onCreated: (sessionId: string) => void
  focusToken?: number
}) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-4 py-8 sm:px-8">
      <QuickSessionComposer onCreated={onCreated} focusToken={focusToken} />
    </div>
  )
}
