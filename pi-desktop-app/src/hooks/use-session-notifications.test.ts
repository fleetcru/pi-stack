import { cleanup, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { useSessionNotifications } from "./use-session-notifications"

const notify = vi.fn()

vi.mock("./use-notifications", () => ({
  useNotifications: () => ({ notify, permissionGranted: true }),
}))

describe("useSessionNotifications", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"))
    notify.mockReset()
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    })
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it("does not notify for an initial snapshot but does notify a later transition", () => {
    const { rerender } = renderHook(
      ({ state }) => useSessionNotifications("1234567890", { state }),
      { initialProps: { state: "idle" } },
    )
    expect(notify).not.toHaveBeenCalled()

    rerender({ state: "waiting_for_input" })
    expect(notify).toHaveBeenCalledWith(
      "Pi needs input",
      "Session 12345678 is waiting for your response.",
    )
  })

  it("does not notify for an initial waiting snapshot", () => {
    renderHook(() => useSessionNotifications("1234567890", { state: "waiting_for_input" }))
    expect(notify).not.toHaveBeenCalled()
  })

  it("notifies for every completed run, including repeated terminal states", () => {
    const { rerender } = renderHook(
      ({ state }) => useSessionNotifications("1234567890", { state }),
      { initialProps: { state: "working" } },
    )

    vi.advanceTimersByTime(5_000)
    rerender({ state: "idle" })
    rerender({ state: "working" })
    vi.advanceTimersByTime(3_000)
    rerender({ state: "idle" })

    expect(notify).toHaveBeenNthCalledWith(
      1,
      "Pi finished",
      "Session 12345678 completed its task after 5s.",
    )
    expect(notify).toHaveBeenNthCalledWith(
      2,
      "Pi finished",
      "Session 12345678 completed its task after 3s.",
    )
  })

  it("resets duration after a failed run", () => {
    const { rerender } = renderHook(
      ({ state }) => useSessionNotifications("1234567890", { state }),
      { initialProps: { state: "working" } },
    )

    vi.advanceTimersByTime(60_000)
    rerender({ state: "failed" })
    rerender({ state: "working" })
    vi.advanceTimersByTime(2_000)
    rerender({ state: "idle" })

    expect(notify).toHaveBeenLastCalledWith(
      "Pi finished",
      "Session 12345678 completed its task after 2s.",
    )
  })

  it("does not expose server detail text", () => {
    const { rerender } = renderHook(
      ({ state, detail }) => useSessionNotifications("1234567890", { state, detail }),
      { initialProps: { state: "working", detail: "C:\\Users\\secret\\file.txt" } },
    )

    rerender({ state: "failed", detail: "C:\\Users\\secret\\file.txt" })

    expect(notify).toHaveBeenCalledWith("Pi session stopped", "Session 12345678 failed.")
  })

  it("does not notify while the window is visible", () => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    })
    const { rerender } = renderHook(
      ({ state }) => useSessionNotifications("1234567890", { state }),
      { initialProps: { state: "working" } },
    )

    rerender({ state: "idle" })
    expect(notify).not.toHaveBeenCalled()
  })
})
