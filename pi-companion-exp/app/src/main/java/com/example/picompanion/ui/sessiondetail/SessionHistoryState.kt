package com.example.picompanion.ui.sessiondetail

/** Owns paged durable-history state and reconciliation with live timeline rows. */
internal class SessionHistoryState {
  var nextOffset: Int = 0
    private set
  var hasOlder: Boolean = false
    private set
  var historicalItems: List<SessionTimelineItem> = emptyList()
    private set

  fun restore(items: List<SessionTimelineItem>, offset: Int, older: Boolean) {
    historicalItems = items
    nextOffset = offset
    hasOlder = older
  }

  fun applyPage(
    page: List<SessionTimelineItem>,
    appendOld: Boolean,
    nextOffset: Int,
    hasOlder: Boolean,
    liveItems: List<SessionTimelineItem>,
    stamp: (SessionTimelineItem) -> SessionTimelineItem,
  ): List<SessionTimelineItem> {
    this.nextOffset = nextOffset
    this.hasOlder = hasOlder
    val previousOrders = (historicalItems + liveItems)
      .filter { it.order > 0 }
      .associate { historyItemId(it) to it.order }
    historicalItems = (if (appendOld) page + historicalItems else page)
      .distinctBy(::historyItemId)
      .map { item -> previousOrders[historyItemId(item)]?.let { item.withOrder(it) } ?: item }

    val merged = LinkedHashMap<String, SessionTimelineItem>()
    historicalItems.forEach { merged[historyItemId(it)] = it }
    liveItems.filter { it is SessionTimelineItem.Chat && it.time == "now" && it.imageUris.isNotEmpty() }
      .forEach { merged[historyItemId(it)] = it }
    liveItems.forEach { item ->
      val optimisticImage = item is SessionTimelineItem.Chat && item.time == "now" && item.imageUris.isNotEmpty()
      if (!optimisticImage) {
        val id = historyItemId(item)
        if (item is SessionTimelineItem.Tool || merged[id] == null) merged[id] = item
      }
    }
    return merged.values.map(stamp)
  }

  private fun SessionTimelineItem.withOrder(order: Long): SessionTimelineItem = when (this) {
    is SessionTimelineItem.Chat -> copy(order = order)
    is SessionTimelineItem.Tool -> copy(order = order)
    is SessionTimelineItem.FileChange -> copy(order = order)
    is SessionTimelineItem.System -> copy(order = order)
  }

  private fun historyItemId(item: SessionTimelineItem): String = when (item) {
    is SessionTimelineItem.Chat -> "chat|${item.isUser}|${item.time}|${item.text.length}|${item.text.hashCode()}|${item.text.take(50)}"
    is SessionTimelineItem.Tool -> "tool|${item.callId}"
    is SessionTimelineItem.FileChange -> "file|${item.operation}|${item.path}"
    is SessionTimelineItem.System -> "system|${item.text}"
  }
}
