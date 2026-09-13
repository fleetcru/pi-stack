import { useEffect, useRef } from "react"
import { useNotifications } from "./use-notifications"

type RuntimeState = {
  state?: string
  detail?: string
}

const isActiveState = (state: string | undefined) =>
  state === "working" || state === "starting" || state === "reconnecting"

/**
 * Watches a session's runtime state and fires generic OS notifications while
 * the desktop window is hidden. Notification bodies avoid runtime details
 * because the operating system may show them on the lock screen.
 */
export function useSessionNotifications(
  sessionId: string,
  runtimeState: RuntimeState | undefined
) {
  const { notify } = useNotifications()
  const currentSession = useRef(sessionId)
  const prevState = useRef<string | undefined>(undefined)
  const workingSince = useRef<number | undefined>(undefined)
  const lastNotification = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (currentSession.current !== sessionId) {
      currentSession.current = sessionId
      prevState.current = undefined
      workingSince.current = undefined
      lastNotification.current = undefined
    }

    const current = runtimeState?.state
    const prev = prevState.current
    prevState.current = current

    if ((current === "working" || current === "starting") && !isActiveState(prev)) {
      workingSince.current = Date.now()
      // A new run may legitimately produce the same terminal state as the last.
      lastNotification.current = undefined
    }
    // Initial snapshots and replay establish baseline state; they are not new
    // transitions and must not produce stale notifications.
    if (prev === undefined) return

    const shouldNotify = typeof document === "undefined" || document.visibilityState === "hidden"
    const elapsed = workingSince.current ? Math.round((Date.now() - workingSince.current) / 1000) : 0
    const duration = elapsed > 0 ? ` after ${elapsed}s` : ""
    const key = `${sessionId}:${current}`
    const sendOnce = (title: string, body: string) => {
      if (!shouldNotify || lastNotification.current === key) return
      lastNotification.current = key
      void notify(title, body)
    }

    if ((prev === "working" || prev === "starting") && (current === "idle" || current === "created")) {
      sendOnce("Pi finished", `Session ${sessionId.slice(0, 8)} completed its task${duration}.`)
      workingSince.current = undefined
    }

    if (current === "waiting_for_input" && prev !== "waiting_for_input") {
      sendOnce("Pi needs input", `Session ${sessionId.slice(0, 8)} is waiting for your response.`)
    }

    if (
      (current === "error" || current === "failed" || current === "stopped") &&
      prev !== current
    ) {
      sendOnce("Pi session stopped", `Session ${sessionId.slice(0, 8)} ${current}.`)
      workingSince.current = undefined
    }
  }, [runtimeState?.state, sessionId, notify])
}
