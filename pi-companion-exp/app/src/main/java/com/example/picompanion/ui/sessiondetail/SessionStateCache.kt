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
    val retainedHistory = entry.historicalItems.takeLast(500)
    entries[key] = entry.copy(
      items = entry.items.takeLast(500),
      historicalItems = retainedHistory,
      nextHistoryOffset = minOf(entry.nextHistoryOffset, retainedHistory.size),
    )
  }
}
