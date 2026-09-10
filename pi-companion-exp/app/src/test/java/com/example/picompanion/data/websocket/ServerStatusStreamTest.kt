package com.example.picompanion.data.websocket

import com.example.picompanion.data.api.PiServerClient
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

class ServerStatusStreamTest {
  @Test
  fun snapshotReplacesStateAndDeltaUpdatesOneSession() {
    val stream = ServerStatusStream(PiServerClient())
    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status_snapshot"))
      put("generation", JsonPrimitive("one"))
      put("cursor", JsonPrimitive(2))
      put("events", buildJsonArray {
        add(statusEvent("s1", "working"))
        add(statusEvent("s2", "idle"))
      })
    })

    assertEquals("working", stream.sessions.value["s1"]?.state)
    assertEquals("idle", stream.sessions.value["s2"]?.state)

    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status"))
      put("generation", JsonPrimitive("one"))
      put("cursor", JsonPrimitive(3))
      put("event", statusEvent("s1", "waiting_for_input"))
    })

    assertEquals("waiting_for_input", stream.sessions.value["s1"]?.state)
    assertEquals("idle", stream.sessions.value["s2"]?.state)
  }

  @Test
  fun newGenerationClearsStatusesBeforeApplyingSnapshot() {
    val stream = ServerStatusStream(PiServerClient())
    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status_snapshot"))
      put("generation", JsonPrimitive("old"))
      put("cursor", JsonPrimitive(1))
      put("events", buildJsonArray { add(statusEvent("old-session", "working")) })
    })
    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status_snapshot"))
      put("generation", JsonPrimitive("new"))
      put("cursor", JsonPrimitive(0))
      put("gap", JsonPrimitive(true))
      put("events", buildJsonArray { })
    })

    assertFalse(stream.sessions.value.containsKey("old-session"))
  }

  @Test
  fun admissionRunDoesNotReplaceWorkingSession() {
    val stream = ServerStatusStream(PiServerClient())
    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status_snapshot"))
      put("generation", JsonPrimitive("one"))
      put("cursor", JsonPrimitive(2))
      put("events", buildJsonArray {
        add(statusEvent("s1", "working"))
        add(buildJsonObject {
          put("type", JsonPrimitive("session_status"))
          put("sessionId", JsonPrimitive("s1"))
          put("workerId", JsonPrimitive("local"))
          put("state", JsonPrimitive("queued"))
          put("reason", JsonPrimitive("admission"))
          put("runId", JsonPrimitive("run-2"))
        })
      })
    })

    assertEquals("working", stream.sessions.value["s1"]?.state)
    assertEquals("queued", stream.activeRuns.value["run-2"]?.state)
  }

  @Test
  fun workerEventsStaySeparateFromSessionStatuses() {
    val stream = ServerStatusStream(PiServerClient())
    stream.handleEnvelope(buildJsonObject {
      put("type", JsonPrimitive("status"))
      put("generation", JsonPrimitive("one"))
      put("cursor", JsonPrimitive(1))
      put("event", buildJsonObject {
        put("type", JsonPrimitive("session_status"))
        put("workerId", JsonPrimitive("worker-1"))
        put("state", JsonPrimitive("offline"))
      })
    })

    assertEquals("offline", stream.workerHealth.value["worker-1"])
    assertNull(stream.sessions.value["worker-1"])
  }

  private fun statusEvent(sessionId: String, state: String) = buildJsonObject {
    put("type", JsonPrimitive("session_status"))
    put("sessionId", JsonPrimitive(sessionId))
    put("workerId", JsonPrimitive("local"))
    put("state", JsonPrimitive(state))
  }
}
