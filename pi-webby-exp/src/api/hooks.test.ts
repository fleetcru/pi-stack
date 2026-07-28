import { describe, it, expect } from "vitest"
import {
  piQueryKeys,
  getSessionDisplayName,
  workerSessionGroups,
} from "./hooks"
import type { ApiSession, ApiWorker } from "./client"

// ---------------------------------------------------------------------------
// piQueryKeys
// ---------------------------------------------------------------------------

describe("piQueryKeys", () => {
  it("health() returns expected tuple", () => {
    const key = piQueryKeys.health("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "health"])
  })

  it("capabilities() returns expected tuple", () => {
    const key = piQueryKeys.capabilities("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "capabilities"])
  })

  it("workers() returns expected tuple", () => {
    const key = piQueryKeys.workers("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "workers"])
  })

  it("sessions() returns expected tuple", () => {
    const key = piQueryKeys.sessions("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "sessions"])
  })

  it("session() returns expected tuple", () => {
    const key = piQueryKeys.session("http://localhost:3141", "abc")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "sessions", "abc"])
  })

  it("sessionData() returns expected tuple", () => {
    const key = piQueryKeys.sessionData("http://localhost:3141", "abc", "messages")
    expect(key).toEqual([
      "pi-server", "http://localhost:3141", "sessions", "abc", "messages",
    ])
  })

  it("git() returns expected tuple", () => {
    const key = piQueryKeys.git("http://localhost:3141", "abc", "status")
    expect(key).toEqual([
      "pi-server", "http://localhost:3141", "sessions", "abc", "git", "status",
    ])
  })

  it("files() returns expected tuple", () => {
    const key = piQueryKeys.files("http://localhost:3141", "/src")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "files", "/src"])
  })

  it("fileContent() returns expected tuple", () => {
    const key = piQueryKeys.fileContent("http://localhost:3141", "abc", "README.md")
    expect(key).toEqual([
      "pi-server", "http://localhost:3141", "sessions", "abc", "file-content", "README.md",
    ])
  })

  it("globalSessions() returns expected tuple", () => {
    const key = piQueryKeys.globalSessions("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "global-sessions"])
  })

  it("machineSessions() returns expected tuple", () => {
    const key = piQueryKeys.machineSessions("http://localhost:3141")
    expect(key).toEqual(["pi-server", "http://localhost:3141", "machine-sessions"])
  })
})

// ---------------------------------------------------------------------------
// getSessionDisplayName
// ---------------------------------------------------------------------------

describe("getSessionDisplayName", () => {
  const base: ApiSession = {
    id: "abc123",
    cwd: "/home/user/project",
    status: "idle",
    workerId: "local",
  } as ApiSession

  it("returns title when present", () => {
    expect(getSessionDisplayName({ ...base, title: "My Title" })).toBe("My Title")
  })

  it("returns project when title is absent", () => {
    expect(getSessionDisplayName({ ...base, project: "my-project" })).toBe("my-project")
  })

  it("returns cwd when title and project are absent", () => {
    expect(getSessionDisplayName(base)).toBe("/home/user/project")
  })

  it("returns id when all others are falsy", () => {
    const session: ApiSession = { ...base, title: undefined, project: undefined, cwd: "" }
    expect(getSessionDisplayName(session)).toBe("abc123")
  })

  it("title takes precedence over project", () => {
    expect(
      getSessionDisplayName({ ...base, title: "Title", project: "proj" })
    ).toBe("Title")
  })
})

// ---------------------------------------------------------------------------
// workerSessionGroups
// ---------------------------------------------------------------------------

describe("workerSessionGroups", () => {
  function makeSession(id: string, workerId: string): ApiSession {
    return {
      id,
      workerId,
      cwd: "/",
      status: "idle",
    } as ApiSession
  }

  const workers: ApiWorker[] = [
    { id: "local", url: "", status: "ok" } as ApiWorker,
    { id: "remote-1", url: "http://r1:3141", status: "ok" } as ApiWorker,
  ]

  it("groups sessions by workerId", () => {
    const sessions = [
      makeSession("s1", "local"),
      makeSession("s2", "local"),
      makeSession("s3", "remote-1"),
    ]

    const groups = workerSessionGroups(sessions, workers)

    expect(groups.size).toBe(2)
    expect(groups.get("local")).toHaveLength(2)
    expect(groups.get("remote-1")).toHaveLength(1)
  })

  it("returns empty map for empty sessions", () => {
    const groups = workerSessionGroups([], workers)
    expect(groups.size).toBe(0)
  })

  it("creates a group for an unknown workerId", () => {
    const sessions = [makeSession("s1", "unknown-worker")]
    const groups = workerSessionGroups(sessions, workers)

    expect(groups.has("unknown-worker")).toBe(true)
    expect(groups.get("unknown-worker")).toHaveLength(1)
  })

  it("handles sessions all on the same worker", () => {
    const sessions = [
      makeSession("s1", "local"),
      makeSession("s2", "local"),
      makeSession("s3", "local"),
    ]

    const groups = workerSessionGroups(sessions, workers)
    expect(groups.size).toBe(1)
    expect(groups.get("local")).toHaveLength(3)
  })
})
