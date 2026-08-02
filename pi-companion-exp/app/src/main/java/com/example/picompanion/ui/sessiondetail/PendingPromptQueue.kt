package com.example.picompanion.ui.sessiondetail

/** Thread-safe FIFO buffer for text prompts created while the session is offline. */
internal class PendingPromptQueue(private val maxSize: Int = 20) {
  private val prompts = ArrayDeque<String>()

  @Synchronized
  fun enqueue(prompt: String): Boolean {
    if (prompts.size >= maxSize) return false
    prompts.addLast(prompt)
    return true
  }

  @Synchronized
  fun dequeue(): String? = if (prompts.isEmpty()) null else prompts.removeFirst()

  @Synchronized
  fun size(): Int = prompts.size
}
