import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { useNotifications } from "./use-notifications"

const { isPermissionGranted, requestPermission, sendNotification } = vi.hoisted(() => ({
  isPermissionGranted: vi.fn(),
  requestPermission: vi.fn(),
  sendNotification: vi.fn(),
}))

vi.mock("@tauri-apps/plugin-notification", () => ({
  isPermissionGranted,
  requestPermission,
  sendNotification,
}))

describe("useNotifications", () => {
  beforeEach(() => {
    isPermissionGranted.mockReset()
    requestPermission.mockReset()
    sendNotification.mockReset()
  })

  it("requests permission on first notification when permission is not already granted", async () => {
    isPermissionGranted.mockResolvedValue(false)
    requestPermission.mockResolvedValue("granted")
    const { result } = renderHook(() => useNotifications())
    await waitFor(() => expect(result.current.permissionGranted).toBe(false))

    await act(async () => {
      await result.current.notify("Pi finished", "Session completed.")
    })

    expect(requestPermission).toHaveBeenCalledTimes(1)
    expect(sendNotification).toHaveBeenCalledWith({
      title: "Pi finished",
      body: "Session completed.",
    })
  })

  it("handles native permission errors without sending", async () => {
    isPermissionGranted.mockResolvedValue(false)
    requestPermission.mockRejectedValue(new Error("native API unavailable"))
    const { result } = renderHook(() => useNotifications())
    await waitFor(() => expect(result.current.permissionGranted).toBe(false))

    await act(async () => {
      await expect(result.current.notify("Pi finished", "Session completed.")).resolves.toBeUndefined()
    })

    expect(sendNotification).not.toHaveBeenCalled()
  })

  it("does not repeatedly request permission after denial", async () => {
    isPermissionGranted.mockResolvedValue(false)
    requestPermission.mockResolvedValue("denied")
    const { result } = renderHook(() => useNotifications())
    await waitFor(() => expect(result.current.permissionGranted).toBe(false))

    await act(async () => {
      await result.current.notify("First", "First")
      await result.current.notify("Second", "Second")
    })

    expect(requestPermission).toHaveBeenCalledTimes(1)
    expect(sendNotification).not.toHaveBeenCalled()
  })
})
