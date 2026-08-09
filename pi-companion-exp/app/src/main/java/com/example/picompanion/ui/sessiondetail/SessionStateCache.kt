package com.example.picompanion.ui.sessiondetail

/** Small process-memory LRU for instant back-and-forth session switching. */
internal object SessionStateCache {
  data class Entry(
    val items: List<SessionTimelineItem>,
    val historicalItems: List<SessionTimelineItem>,
    val nextHistoryOffset: Int,
    val hasOlder: Boolean,
    val title: String,
    val project: String,
    val cwd: String,
    val lastEventId: Long,
  )

  private const val MaxEntries = 5
  private val entries = object : LinkedHashMap<String, Entry>(MaxEntries, 0.75f, true) {
    override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Entry>?): Boolean = size > MaxEntries
  }

  @Synchronized fun get(key: String): Entry? = entries[key]
  @Synchronized fun contains(key: String): Boolean = entries.containsKey(key)
  @Synchronized fun put(key: String, entry: Entry) {
    val assigned = mutableMapOf<String, Long>()
    var nextOrder = (entry.historicalItems + entry.items).maxOfOrNull { it.order } ?: 0
    fun normalize(items: List<SessionTimelineItem>): List<SessionTimelineItem> = items.map { item ->
      if (item.order > 0) {
        assigned[item.cacheId()] = item.order
        item
      } else {
        val order = assigned[item.cacheId()] ?: run {
          nextOrder += 1
          nextOrder.also { assigned[item.cacheId()] = it }
        }
        item.withOrder(order)
      }
    }

    val retainedHistory = normalize(entry.historicalItems.takeLast(500))
    val retainedItems = normalize(entry.items.takeLast(500))
    entries[key] = entry.copy(
      items = retainedItems,
      historicalItems = retainedHistory,
      nextHistoryOffset = minOf(entry.nextHistoryOffset, retainedHistory.size),
    )
  }

  private fun SessionTimelineItem.withOrder(order: Long): SessionTimelineItem = when (this) {
    is SessionTimelineItem.Chat -> copy(order = order)
    is SessionTimelineItem.Tool -> copy(order = order)
    is SessionTimelineItem.FileChange -> copy(order = order)
    is SessionTimelineItem.System -> copy(order = order)
  }

  private fun SessionTimelineItem.cacheId(): String = when (this) {
    is SessionTimelineItem.Chat -> "chat|$isUser|$time|${text.length}|${text.hashCode()}|${text.take(50)}"
    is SessionTimelineItem.Tool -> "tool|$callId"
    is SessionTimelineItem.FileChange -> "file|$operation|$path"
    is SessionTimelineItem.System -> "system|$text"
  }
}
