import { useCallback, useEffect, useRef, useState } from "react"
import { isPermissionGranted, requestPermission, sendNotification } from "@tauri-apps/plugin-notification"

/**
 * Hook for sending OS-native notifications from the desktop app.
 * Requests permission on first use. Sends notifications for session events.
 */
export function useNotifications() {
  const [permissionGranted, setPermissionGranted] = useState<boolean | null>(null)
  const permissionRequestAttempted = useRef(false)

  useEffect(() => {
    let cancelled = false
    void isPermissionGranted()
      .then((granted) => {
        if (!cancelled && !permissionRequestAttempted.current) setPermissionGranted(granted)
      })
      .catch(() => {
        if (!cancelled && !permissionRequestAttempted.current) setPermissionGranted(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const ensurePermission = useCallback(async () => {
    if (permissionGranted === true) return true
    if (permissionRequestAttempted.current) return false
    permissionRequestAttempted.current = true
    try {
      const result = await requestPermission()
      const granted = result === "granted"
      setPermissionGranted(granted)
      return granted
    } catch {
      setPermissionGranted(false)
      return false
    }
  }, [permissionGranted])

  const notify = useCallback(async (title: string, body: string) => {
    const granted = await ensurePermission()
    if (!granted) return
    try {
      await sendNotification({ title, body })
    } catch {
      // Native notification failures must not affect the session UI.
    }
  }, [ensurePermission])

  return { notify, permissionGranted }
}
