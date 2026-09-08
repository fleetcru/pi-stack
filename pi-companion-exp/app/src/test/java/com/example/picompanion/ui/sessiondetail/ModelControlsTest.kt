package com.example.picompanion.ui.sessiondetail

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ModelControlsTest {
  @Test
  fun parsesSessionAndGlobalResponseShapes() {
    val sessionPayload = Json.parseToJsonElement(
      """{"data":{"models":[{"provider":"openai","id":"gpt-5","name":"GPT 5"}]}}""",
    ).jsonObject
    val globalPayload = Json.parseToJsonElement(
      """{"models":[{"provider":"google","id":"gemini-2.5-pro"}]}""",
    ).jsonObject

    assertEquals(
      listOf(ModelChoice("openai", "gpt-5", "GPT 5")),
      parseModelChoices(sessionPayload),
    )
    assertEquals(
      listOf(ModelChoice("google", "gemini-2.5-pro", "gemini-2.5-pro")),
      parseModelChoices(globalPayload),
    )
  }

  @Test
  fun skipsMalformedModelsWithoutDroppingValidProviders() {
    val payload = Json.parseToJsonElement(
      """{"models":[{"provider":"","id":"bad"},{"provider":"anthropic"},{"provider":"fireworks","id":"accounts/fireworks/models/kimi"}]}""",
    ).jsonObject

    assertEquals(
      listOf(ModelChoice("fireworks", "accounts/fireworks/models/kimi", "accounts/fireworks/models/kimi")),
      parseModelChoices(payload),
    )
  }

  @Test
  fun mergesSourcesWithoutErasingOrDuplicatingModels() {
    val session = listOf(ModelChoice("openai", "gpt-5", "GPT 5"))
    val global = listOf(
      ModelChoice("openai", "gpt-5", "duplicate"),
      ModelChoice("google", "gemini-2.5-pro", "Gemini"),
    )
    val eventModels = listOf(ModelChoice("anthropic", "claude-sonnet", "Claude Sonnet"))

    val merged = mergeModelChoices(session, global, eventModels)

    assertEquals(3, merged.size)
    assertEquals("GPT 5", merged.first().name)
    assertTrue(merged.any { it.provider == "google" })
    assertTrue(merged.any { it.provider == "anthropic" })
  }
}
