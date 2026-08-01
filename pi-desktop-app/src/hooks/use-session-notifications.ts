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

  useEffect(() => {
    const current = runtimeState?.state
    const prev = prevState.current
    prevState.current = current

    // Transition: working → idle/created → Pi finished a task
    if (prev === "working" && (current === "idle" || current === "created")) {
      notify("Pi finished", `Session ${sessionId.slice(0, 8)} completed its task.`)
    }

    // Transition: * → waiting_for_input → Pi needs your input
    if (current === "waiting_for_input" && prev !== "waiting_for_input") {
      notify("Pi needs input", `Session ${sessionId.slice(0, 8)} is waiting for your response.`)
    }

    // Transition: * → error/failed/stopped → something went wrong
    if (
      (current === "error" || current === "failed" || current === "stopped") &&
      prev !== current
    ) {
      notify("Pi session stopped", `Session ${sessionId.slice(0, 8)} ${current}.`)
    }
  }, [runtimeState?.state, sessionId, notify])
}
