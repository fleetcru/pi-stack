package com.example.picompanion.data.model

import com.example.picompanion.data.api.apiJson
import kotlinx.serialization.encodeToString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SessionInventoryCacheTest {
  @Test
  fun decodesCacheForMatchingServer() {
    val encoded = apiJson.encodeToString(SessionInventoryCache(serverId = "server-a"))

    val cache = decodeSessionInventoryCache(encoded, "server-a", apiJson)

    assertEquals("server-a", cache?.serverId)
  }

  @Test
  fun rejectsCacheForDifferentServer() {
    val encoded = apiJson.encodeToString(SessionInventoryCache(serverId = "server-a"))

    val cache = decodeSessionInventoryCache(encoded, "server-b", apiJson)

    assertNull(cache)
  }

  @Test
  fun rejectsMalformedCache() {
    assertNull(decodeSessionInventoryCache("not json", "server-a", apiJson))
  }
}
