package com.example.picompanion.ui.sessiondetail

/** Owns paged durable-history state and reconciliation with live timeline rows. */
internal class SessionHistoryState {
  var nextOffset: Int = 0
    private set
  var hasOlder: Boolean = false
    private set
  var historicalItems: List<SessionTimelineItem> = emptyList()
    private set
  var totalMessages: Int = 0
    private set

  fun restore(
    items: List<SessionTimelineItem>,
    offset: Int,
    older: Boolean,
    totalMessages: Int = 0,
  ) {
    historicalItems = items
    nextOffset = offset
    hasOlder = older
    this.totalMessages = totalMessages
  }

  fun retainTotalMessages(totalMessages: Int) {
    this.totalMessages = maxOf(this.totalMessages, totalMessages)
  }

  fun applyPage(
    page: List<SessionTimelineItem>,
    appendOld: Boolean,
    nextOffset: Int,
    hasOlder: Boolean,
    liveItems: List<SessionTimelineItem>,
    stamp: (SessionTimelineItem) -> SessionTimelineItem,
    totalMessages: Int = this.totalMessages,
    pageStartIndex: Int? = null,
  ): List<SessionTimelineItem> {
    val previousTotalMessages = this.totalMessages
    this.nextOffset = nextOffset
    this.hasOlder = hasOlder
    this.totalMessages = totalMessages
    val previousOrders = (historicalItems + liveItems)
      .filter { it.order > 0 }
      .associate { historyItemId(it) to it.order }

    // The API returns each page oldest-to-newest. Older-page loads prepend.
    // A newest-page refresh keeps any already-loaded prefix before the first
    // overlapping row, then replaces the overlapping tail with fresh data.
    // This preserves transcript chronology without relying on local arrival
    // order, which can put live rows before history when HTTP finishes late.
    val previousHistory = historicalItems
    var resetForHistoryGap = false
    val refreshedHistory = if (appendOld) {
      page + previousHistory
    } else {
      val pageIds = page.mapTo(HashSet(), ::historyItemId)
      val firstOverlap = previousHistory.indexOfFirst { historyItemId(it) in pageIds }
      // Absolute message totals are stable when Pi appends to a transcript.
      // If the newest page starts after the previous transcript end, more
      // records arrived than this page can bridge and the old/new ranges have
      // a real gap. Restart from the contiguous newest range so pagination can
      // fill it instead of presenting old rows in the wrong order.
      resetForHistoryGap = firstOverlap < 0 &&
        previousHistory.isNotEmpty() &&
        previousTotalMessages > 0 &&
        pageStartIndex != null &&
        pageStartIndex > previousTotalMessages
      val olderPrefix = when {
        previousHistory.isEmpty() || resetForHistoryGap -> emptyList()
        firstOverlap >= 0 -> previousHistory.take(firstOverlap)
        else -> previousHistory
      }
      olderPrefix + page
    }
    historicalItems = refreshedHistory
      .distinctBy(::historyItemId)
      .map { item -> previousOrders[historyItemId(item)]?.let { item.withOrder(it) } ?: item }

    val unmatchedOptimisticImages = liveItems.filterIsInstance<SessionTimelineItem.Chat>()
      .filter { it.time == "now" && it.imageUris.isNotEmpty() }
      .toMutableList()
    val merged = LinkedHashMap<String, SessionTimelineItem>()
    historicalItems.forEach { item ->
      val durable = item as? SessionTimelineItem.Chat
      val previewIndex = durable?.let {
        unmatchedOptimisticImages.indexOfFirst { optimistic ->
          optimistic.isUser == durable.isUser && optimistic.text.trim() == durable.text.trim()
        }
      } ?: -1
      val withLocalPreview = if (durable != null && previewIndex >= 0) {
        durable.copy(imageUris = unmatchedOptimisticImages.removeAt(previewIndex).imageUris)
      } else {
        item
      }
      merged[historyItemId(withLocalPreview)] = withLocalPreview
    }
    // Match durable image echoes one-to-one. Repeated prompts with the same
    // caption must not cause every optimistic preview to disappear at once.
    unmatchedOptimisticImages.forEach { merged[historyItemId(it)] = it }

    val liveItemIds = liveItems.mapTo(mutableSetOf(), ::historyItemId)
    val newlyDurableChats = historicalItems
      .asSequence()
      .filterIsInstance<SessionTimelineItem.Chat>()
      // Only newly durable rows can replace ephemeral rows. Existing history
      // may contain an older, identical response from a different turn.
      .filter { !appendOld && historyItemId(it) !in liveItemIds }
      .toList()
    val newestDurableAssistant = newlyDurableChats.lastOrNull { !it.isUser }
    val unmatchedDurableChats = newlyDurableChats.toMutableList()
    val reconciledLiveText = BooleanArray(liveItems.size)
    liveItems.forEachIndexed { index, item ->
      val chat = item as? SessionTimelineItem.Chat ?: return@forEachIndexed
      val ephemeralText = chat.time.isEmpty() || chat.time == "now"
      if (!ephemeralText || chat.imageUris.isNotEmpty()) return@forEachIndexed
      val durableIndex = unmatchedDurableChats.indexOfFirst { durable ->
        durable.replacesEphemeral(chat, allowPrefix = durable === newestDurableAssistant)
      }
      if (durableIndex >= 0) {
        reconciledLiveText[index] = true
        unmatchedDurableChats.removeAt(durableIndex)
      }
    }
    liveItems.forEachIndexed { index, item ->
      if (resetForHistoryGap && historySourceIndex(item) != null) return@forEachIndexed
      val chat = item as? SessionTimelineItem.Chat
      val optimisticImage = chat?.time == "now" && chat.imageUris.isNotEmpty()
      if (!optimisticImage && !reconciledLiveText[index]) {
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
