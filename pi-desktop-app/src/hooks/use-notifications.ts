import { useCallback, useEffect, useState } from "react"
import { isPermissionGranted, requestPermission, sendNotification } from "@tauri-apps/plugin-notification"

/**
 * Hook for sending OS-native notifications from the desktop app.
 * Requests permission on first use. Sends notifications for session events.
 */
export function useNotifications() {
  const [permissionGranted, setPermissionGranted] = useState<boolean | null>(null)

  useEffect(() => {
    // Check existing permission on mount
    isPermissionGranted().then(setPermissionGranted)
  }, [])

  const ensurePermission = useCallback(async () => {
    if (permissionGranted === true) return true
    if (permissionGranted === false) return false
    // Not yet checked or unknown — request
    const result = await requestPermission()
    const granted = result === "granted"
    setPermissionGranted(granted)
    return granted
  }, [permissionGranted])

  const notify = useCallback(async (title: string, body: string) => {
    const granted = await ensurePermission()
    if (!granted) return
    sendNotification({ title, body })
  }, [ensurePermission])

  return { notify, permissionGranted }
}
