import { useLayoutEffect } from "react"
import { Navigate, Route, Routes, useParams } from "react-router"

import { WorkspaceShell } from "@/components/workspace-shell"
import { useAppStore } from "@/state/app-store"
import "./init-shared"

function SessionRoute() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const selectSession = useAppStore((state) => state.selectSession)
  // Synchronize restored state before paint so refreshing a session URL never
  // briefly mounts the previously persisted workspace.
  useLayoutEffect(() => { selectSession(sessionId) }, [selectSession, sessionId])
  return <WorkspaceShell />
}

function RootRoute() {
  const selectSession = useAppStore((state) => state.selectSession)
  // Clear a restored selection before paint when opening the root route.
  useLayoutEffect(() => { selectSession() }, [selectSession])
  return <WorkspaceShell />
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<RootRoute />} />
      <Route path="/sessions/:sessionId" element={<SessionRoute />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
