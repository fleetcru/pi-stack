package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PendingPromptQueueTest {

  @Test
  fun offlinePromptsAreDeliveredInFirstInFirstOutOrderAfterReconnect() {
    val queue = PendingPromptQueue()
    assertTrue(queue.enqueue("first"))
    assertTrue(queue.enqueue("second"))

    assertEquals("first", queue.dequeue())
    assertEquals("second", queue.dequeue())
    assertNull(queue.dequeue())
  }

  @Test
  fun queueRejectsPromptsAfterCapacityWithoutDroppingQueuedMessages() {
    val queue = PendingPromptQueue(maxSize = 2)
    assertTrue(queue.enqueue("first"))
    assertTrue(queue.enqueue("second"))
    assertFalse(queue.enqueue("third"))

    assertEquals(2, queue.size())
    assertEquals("first", queue.dequeue())
    assertEquals("second", queue.dequeue())
  }
}
