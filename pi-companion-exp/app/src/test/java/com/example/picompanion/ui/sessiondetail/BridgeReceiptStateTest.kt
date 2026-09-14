package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class BridgeReceiptStateTest {
  @Test
  fun ignoresReceiptsWhenNoPromptIsPending() {
    assertNull(bridgeReceiptSendState(status = "delivered", hasPendingPrompt = false))
    assertNull(bridgeReceiptSendState(status = "failed", hasPendingPrompt = false))
  }

  @Test
  fun appliesReceiptsToPendingPrompts() {
    assertEquals(
      SendState.Delivered,
      bridgeReceiptSendState(status = "delivered", hasPendingPrompt = true),
    )
    assertEquals(
      SendState.Failed("The TUI could not accept this message"),
      bridgeReceiptSendState(status = "failed", hasPendingPrompt = true),
    )
  }
}
