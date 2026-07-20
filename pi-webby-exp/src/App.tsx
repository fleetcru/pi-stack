import { useEffect } from "react"
import { Navigate, Route, Routes, useParams } from "react-router"

import { WorkspaceShell } from "@/components/workspace-shell"
import { useAppStore } from "@/state/app-store"

function SessionRoute() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const selectSession = useAppStore((state) => state.selectSession)
  useEffect(() => { selectSession(sessionId) }, [selectSession, sessionId])
  return <WorkspaceShell />
}

function RootRoute() {
  const selectSession = useAppStore((state) => state.selectSession)
  useEffect(() => { selectSession() }, [selectSession])
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
