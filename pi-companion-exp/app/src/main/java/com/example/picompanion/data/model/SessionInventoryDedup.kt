package com.example.picompanion.data.model

fun visibleMachineSessions(
  activeSessions: List<ServerSession>,
  machineSessions: List<MachineSession>,
): List<MachineSession> {
  val activeIds = activeSessions.mapTo(mutableSetOf()) { it.id }
  return machineSessions.filterNot { it.serverSessionId != null && it.serverSessionId in activeIds }
}

fun visibleGlobalSessions(
  activeSessions: List<ServerSession>,
  globalSessions: List<GlobalSession>,
): List<GlobalSession> {
  val activeIds = activeSessions.mapTo(mutableSetOf()) { it.id }
  return globalSessions.filterNot { it.workerId == "local" && it.originId in activeIds }
}
