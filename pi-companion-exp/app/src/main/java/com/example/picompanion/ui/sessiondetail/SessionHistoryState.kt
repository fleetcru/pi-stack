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

    // The API returns each page oldest-to-newest. Older-page loads prepend.
    // A newest-page refresh keeps any already-loaded prefix before the first
    // overlapping row, then replaces the overlapping tail with fresh data.
    // This preserves transcript chronology without relying on local arrival
    // order, which can put live rows before history when HTTP finishes late.
    val previousHistory = historicalItems
    val refreshedHistory = if (appendOld) {
      page + previousHistory
    } else {
      val pageIds = page.mapTo(HashSet(), ::historyItemId)
      val firstOverlap = previousHistory.indexOfFirst { historyItemId(it) in pageIds }
      val olderPrefix = when {
        previousHistory.isEmpty() -> emptyList()
        firstOverlap >= 0 -> previousHistory.take(firstOverlap)
        else -> previousHistory
      }
      olderPrefix + page
    }
    historicalItems = refreshedHistory
      .distinctBy(::historyItemId)
      .map { item -> previousOrders[historyItemId(item)]?.let { item.withOrder(it) } ?: item }

    val optimisticImages = liveItems.filterIsInstance<SessionTimelineItem.Chat>()
      .filter { it.time == "now" && it.imageUris.isNotEmpty() }
    val merged = LinkedHashMap<String, SessionTimelineItem>()
    historicalItems.forEach { item ->
      val withLocalPreview = (item as? SessionTimelineItem.Chat)?.let { durable ->
        optimisticImages.firstOrNull { optimistic ->
          optimistic.isUser == durable.isUser && optimistic.text.trim() == durable.text.trim()
        }?.let { optimistic -> durable.copy(imageUris = optimistic.imageUris) }
      } ?: item
      merged[historyItemId(withLocalPreview)] = withLocalPreview
    }
    // Keep an optimistic image row when its durable echo has not arrived yet.
    optimisticImages.filter { optimistic ->
      historicalItems.none { durable ->
        durable is SessionTimelineItem.Chat &&
          durable.isUser == optimistic.isUser && durable.text.trim() == optimistic.text.trim()
      }
    }.forEach { merged[historyItemId(it)] = it }

    val liveItemIds = liveItems.mapTo(mutableSetOf(), ::historyItemId)
    val newlyDurableChats = historicalItems
      .asSequence()
      .filterIsInstance<SessionTimelineItem.Chat>()
      // Only newly durable rows can replace ephemeral rows. Existing history
      // may contain an older, identical response from a different turn.
      .filter { !appendOld && historyItemId(it) !in liveItemIds }
      .toList()
    val newestDurableAssistant = newlyDurableChats.lastOrNull { !it.isUser }
    liveItems.forEach { item ->
      val chat = item as? SessionTimelineItem.Chat
      val optimisticImage = chat?.time == "now" && chat.imageUris.isNotEmpty()
      val ephemeralText = chat != null && (chat.time.isEmpty() || chat.time == "now")
      val reconciledLiveText = chat != null && ephemeralText && chat.imageUris.isEmpty() &&
        newlyDurableChats.any { durable ->
          durable.replacesEphemeral(chat, allowPrefix = durable === newestDurableAssistant)
        }
      if (!optimisticImage && !reconciledLiveText) {
        val id = historyItemId(item)
        if (item is SessionTimelineItem.Tool || merged[id] == null) merged[id] = item
      }
    }

    // LinkedHashMap order is transcript history followed by live-only rows.
    // Numeric order is a stable item identity, not the source of chronology:
    // sorting by it would move history fetched after a live event to the end.
    val stamped = merged.values.map(stamp)
    val assignedOrders = stamped.associate { historyItemId(it) to it.order }
    historicalItems = historicalItems.map { item ->
      assignedOrders[historyItemId(item)]?.let { item.withOrder(it) } ?: item
    }
    return stamped
  }

  private fun SessionTimelineItem.Chat.replacesEphemeral(
    ephemeral: SessionTimelineItem.Chat,
    allowPrefix: Boolean,
  ): Boolean {
    if (isUser != ephemeral.isUser) return false
    val durableText = text.trim()
    val ephemeralText = ephemeral.text.trim()
    if (durableText == ephemeralText) return true
    // A history refresh can observe the completed response while the live row
    // still contains an earlier streamed prefix. Require a useful prefix size
    // to avoid matching an unrelated older response such as "Done".
    return allowPrefix && !isUser && ephemeralText.length >= 32 && durableText.startsWith(ephemeralText)
  }

  private fun SessionTimelineItem.withOrder(order: Long): SessionTimelineItem = when (this) {
    is SessionTimelineItem.Chat -> copy(order = order)
    is SessionTimelineItem.Tool -> copy(order = order)
    is SessionTimelineItem.FileChange -> copy(order = order)
    is SessionTimelineItem.System -> copy(order = order)
  }

  private fun historyItemId(item: SessionTimelineItem): String = when (item) {
    is SessionTimelineItem.Chat -> "chat|${item.sourceId ?: "${item.isUser}|${item.time}|${item.text.length}|${item.text.hashCode()}|${item.text.take(50)}"}"
    is SessionTimelineItem.Tool -> "tool|${item.callId}"
    is SessionTimelineItem.FileChange -> "file|${item.operation}|${item.path}"
    is SessionTimelineItem.System -> "system|${item.text}"
  }
}
