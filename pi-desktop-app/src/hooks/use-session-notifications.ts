import { useEffect, useRef } from "react"
import { useNotifications } from "./use-notifications"

type RuntimeState = {
  state?: string
  detail?: string
}

/**
 * Watches a session's runtime state and fires OS notifications when the
 * session transitions from "working" → "idle" (i.e., Pi finished a task).
 */
export function useSessionNotifications(
  sessionId: string,
  runtimeState: RuntimeState | undefined
) {
  const { notify } = useNotifications()
  const prevState = useRef<string | undefined>(undefined)
  const workingSince = useRef<number | undefined>(undefined)
  const lastNotification = useRef<string | undefined>(undefined)

  useEffect(() => {
    const current = runtimeState?.state
    const prev = prevState.current
    prevState.current = current
    if (current === "working" || current === "starting" || current === "reconnecting") {
      workingSince.current ??= Date.now()
    }

    // Native notifications are useful when the desktop window is not visible;
    // avoid interrupting the user while they are already watching the session.
    const shouldNotify = typeof document === "undefined" || document.visibilityState === "hidden"
    const elapsed = workingSince.current ? Math.round((Date.now() - workingSince.current) / 1000) : 0
    const duration = elapsed > 0 ? ` after ${elapsed}s` : ""
    const detail = runtimeState?.detail?.trim()
    const key = `${current}:${detail ?? ""}`
    const sendOnce = (title: string, body: string) => {
      if (!shouldNotify || lastNotification.current === key) return
      lastNotification.current = key
      void notify(title, body)
    }

    // Transition: working → idle/created → Pi finished a task
    if (prev === "working" && (current === "idle" || current === "created")) {
      sendOnce("Pi finished", `Session ${sessionId.slice(0, 8)} completed its task${duration}.`)
      workingSince.current = undefined
    }

    // Transition: * → waiting_for_input → Pi needs your input
    if (current === "waiting_for_input" && prev !== "waiting_for_input") {
      sendOnce("Pi needs input", detail || `Session ${sessionId.slice(0, 8)} is waiting for your response.`)
    }

    // Transition: * → error/failed/stopped → something went wrong
    if (
      (current === "error" || current === "failed" || current === "stopped") &&
      prev !== current
    ) {
      sendOnce("Pi session stopped", detail || `Session ${sessionId.slice(0, 8)} ${current}.`)
    }
  }, [runtimeState?.state, sessionId, notify])
}
