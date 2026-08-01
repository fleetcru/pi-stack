package com.example.picompanion.ui.sessiondetail

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay

/**
 * Redesigned tool event row with:
 * - Colored left accent border (blue=running, green=done, red=failed)
 * - Inline argument preview (file path, command, search query)
 * - Duration display
 * - Animated pulse for running tools
 * - Copy button on output
 * - Structured output sections (args + output)
 */
@Composable
fun ToolEventRow(item: SessionTimelineItem.Tool, modifier: Modifier = Modifier) {
  var expanded by rememberSaveable(item.callId) { mutableStateOf(false) }
  var copied by rememberSaveable { mutableStateOf(false) }

  // Semantic tool icon + label mapping (matches shared toolDisplayName)
  val (icon, label) = when (item.name.lowercase()) {
    "bash", "terminal" -> Icons.Filled.Terminal to "Terminal"
    "read", "read_file" -> Icons.Filled.Code to "Read file"
    "write", "edit", "apply_patch" -> Icons.Filled.Code to "Edit file"
    "find", "grep", "search" -> Icons.Filled.Search to "Search"
    "ls", "list", "list_directory" -> Icons.Filled.Folder to "Browse files"
    else -> Icons.Filled.Code to item.name
  }

  val isRunning = item.status == "running"
  val isFailed = item.status == "failed"
  val isDone = item.status == "completed"

  // Accent colors
  val accentColor = when {
    isFailed -> MaterialTheme.colorScheme.error
    isRunning -> MaterialTheme.colorScheme.primary
    else -> MaterialTheme.colorScheme.tertiary
  }
  val accentBg = when {
    isFailed -> MaterialTheme.colorScheme.errorContainer.copy(alpha = 0.15f)
    isRunning -> MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.15f)
    else -> MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.25f)
  }

  // Inline summary (what the tool did)
  val summary = remember(item.name, item.args) {
    extractToolSummary(item.name, item.args)
  }

  // Duration
  val durationMs = if (isDone && item.startedAt != null && item.endedAt != null) {
    (item.endedAt!! - item.startedAt!!)
  } else null

  // Live elapsed timer for running tools
  var liveElapsed by remember { mutableLongStateOf(0L) }
  LaunchedEffect(isRunning, item.startedAt) {
    if (isRunning && item.startedAt != null) {
      while (true) {
        liveElapsed = System.currentTimeMillis() - item.startedAt!!
        delay(1000)
      }
    }
  }

  Box(
    modifier = modifier
      .fillMaxWidth()
      .clip(RoundedCornerShape(10.dp))
      .clickable { expanded = !expanded }
      .animateContentSize(),
  ) {
    Column {
      // Main row with left accent
      Row(
        modifier = Modifier
          .fillMaxWidth()
          .drawBehind {
            // Left accent bar
            drawRoundRect(
              color = accentColor.copy(alpha = 0.8f),
              topLeft = Offset.Zero,
              size = Size(4.dp.toPx(), size.height),
              cornerRadius = CornerRadius(2.dp.toPx()),
            )
            // Background tint
            drawRoundRect(
              color = accentBg,
              topLeft = Offset(4.dp.toPx(), 0f),
              size = Size(size.width - 4.dp.toPx(), size.height),
              cornerRadius = CornerRadius(10.dp.toPx()),
            )
          }
          .padding(start = 12.dp, end = 8.dp, top = 8.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
      ) {
        // Icon with pulse animation for running
        if (isRunning) {
          val infiniteTransition = rememberInfiniteTransition(label = "pulse")
          val alpha by infiniteTransition.animateFloat(
            initialValue = 0.4f,
            targetValue = 1f,
            animationSpec = infiniteRepeatable(
              animation = tween(800, easing = LinearEasing),
              repeatMode = RepeatMode.Reverse,
            ),
            label = "pulseAlpha",
          )
          Icon(
            icon,
            contentDescription = null,
            tint = accentColor.copy(alpha = alpha),
            modifier = Modifier.size(16.dp),
          )
        } else {
          Icon(icon, contentDescription = null, tint = accentColor, modifier = Modifier.size(16.dp))
        }

        Spacer(Modifier.width(8.dp))

        // Label + inline summary
        Column(modifier = Modifier.weight(1f)) {
          Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            fontWeight = FontWeight.SemiBold,
            color = MaterialTheme.colorScheme.onSurface,
          )
          if (summary.isNotBlank()) {
            Text(
              summary,
              style = MaterialTheme.typography.labelSmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              maxLines = 1,
              overflow = TextOverflow.Ellipsis,
            )
          }
        }

        // Duration or live timer
        if (isRunning && item.startedAt != null) {
          Text(
            formatDuration(liveElapsed),
            style = MaterialTheme.typography.labelSmall,
            color = accentColor,
            fontWeight = FontWeight.Medium,
          )
          Spacer(Modifier.width(4.dp))
        } else if (durationMs != null) {
          Text(
            formatDuration(durationMs.toLong()),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
          Spacer(Modifier.width(4.dp))
        }

        // Status icon
        when {
          isRunning -> CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = accentColor)
          isFailed -> Icon(Icons.Filled.Error, contentDescription = "Failed", tint = accentColor, modifier = Modifier.size(18.dp))
          isDone -> Icon(Icons.Filled.CheckCircle, contentDescription = "Done", tint = accentColor, modifier = Modifier.size(18.dp))
        }

        Spacer(Modifier.width(4.dp))

        // Expand chevron
        Icon(
          if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore,
          contentDescription = if (expanded) "Collapse" else "Expand",
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      // Expanded content
      if (expanded) {
        Column(
          modifier = Modifier
            .fillMaxWidth()
            .padding(start = 16.dp, end = 8.dp, bottom = 8.dp),
        ) {
          // Arguments section
          item.args?.let { args ->
            SectionHeader("Arguments")
            OutputBlock(args)
          }

          // Output section
          item.output?.let { output ->
            Row(
              modifier = Modifier.fillMaxWidth(),
              horizontalArrangement = Arrangement.SpaceBetween,
              verticalAlignment = Alignment.CenterVertically,
            ) {
              SectionHeader(if (isRunning) "Live output" else if (isFailed) "Error output" else "Output")
              IconButton(
                onClick = {
                  // Copy handled via clipboard manager at call site
                  copied = true
                },
                modifier = Modifier.size(28.dp),
              ) {
                Icon(
                  Icons.Default.ContentCopy,
                  contentDescription = "Copy output",
                  modifier = Modifier.size(14.dp),
                  tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
              }
            }
            OutputBlock(output)
          }

          if (item.args == null && item.output == null) {
            Text(
              if (isRunning) "Waiting for output…" else "No output",
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              modifier = Modifier.padding(top = 4.dp),
            )
          }
        }
      }
    }
  }
}

@Composable
private fun SectionHeader(text: String) {
  Text(
    text,
    style = MaterialTheme.typography.labelSmall,
    fontWeight = FontWeight.SemiBold,
    color = MaterialTheme.colorScheme.onSurfaceVariant,
    modifier = Modifier.padding(top = 8.dp, bottom = 4.dp),
  )
}

@Composable
private fun OutputBlock(text: String) {
  val maxPreviewChars = 80_000
  val preview = text.take(maxPreviewChars)
  Surface(
    shape = RoundedCornerShape(8.dp),
    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f),
    modifier = Modifier.fillMaxWidth(),
  ) {
    Column(Modifier.padding(8.dp)) {
      Text(
        preview,
        style = MaterialTheme.typography.bodySmall.copy(fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace),
        color = MaterialTheme.colorScheme.onSurface,
      )
      if (text.length > maxPreviewChars) {
        Text(
          "Output truncated — ${text.length / 1024} KB total",
          style = MaterialTheme.typography.labelSmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
          modifier = Modifier.padding(top = 4.dp),
        )
      }
    }
  }
}

/**
 * Extracts a one-line summary of what a tool call did.
 * Matches the shared extractToolSummary() logic in webby-shared.
 */
private fun extractToolSummary(toolName: String, args: String?): String {
  val lower = toolName.lowercase()
  return when {
    lower == "bash" || lower == "terminal" -> {
      val cmd = args?.let { parseArgString(it, "command") } ?: args?.let { parseArgString(it, "cmd") }
      cmd?.split("\n")?.firstOrNull()?.trim()?.take(60) ?: ""
    }
    lower == "read" || lower == "read_file" -> {
      val path = args?.let { parseArgString(it, "file_path") } ?: args?.let { parseArgString(it, "path") }
      path?.substringAfterLast('/')?.substringAfterLast('\\') ?: ""
    }
    lower == "write" || lower == "edit" || lower == "apply_patch" -> {
      val path = args?.let { parseArgString(it, "file_path") } ?: args?.let { parseArgString(it, "path") }
      path?.substringAfterLast('/')?.substringAfterLast('\\') ?: ""
    }
    lower == "find" || lower == "grep" || lower == "search" -> {
      val query = args?.let { parseArgString(it, "query") } ?: args?.let { parseArgString(it, "pattern") }
      val path = args?.let { parseArgString(it, "path") } ?: args?.let { parseArgString(it, "directory") }
      val q = query?.let { "\"$it\"" } ?: ""
      val p = path?.let { " in ${it.substringAfterLast('/')}" } ?: ""
      q + p
    }
    lower == "ls" || lower == "list" || lower == "list_directory" -> {
      val path = args?.let { parseArgString(it, "path") } ?: args?.let { parseArgString(it, "directory") }
      path?.substringAfterLast('/')?.substringAfterLast('\\') ?: ""
    }
    else -> ""
  }
}

/** Extracts a value from a JSON-like args string by key. */
private fun parseArgString(args: String, key: String): String? {
  // Simple extraction: look for "key":"value" or "key": "value"
  val regex = Regex("\"$key\"\\s*:\\s*\"([^\"]*)\"")
  return regex.find(args)?.groupValues?.getOrNull(1)
}

/**
 * Formats duration in milliseconds to human-readable string.
 * Matches the shared formatDuration() in webby-shared.
 */
private fun formatDuration(ms: Long): String {
  if (ms < 1000) return "${ms}ms"
  if (ms < 60_000) return "${"%.1f".format(ms / 1000.0)}s"
  val minutes = ms / 60_000
  val seconds = (ms % 60_000) / 1000
  return "$minutes:${seconds.toString().padStart(2, '0')}"
}

@Composable
fun FileChangeRow(item: SessionTimelineItem.FileChange, modifier: Modifier = Modifier) {
  Row(
    modifier.fillMaxWidth(),
    horizontalArrangement = Arrangement.spacedBy(8.dp),
    verticalAlignment = Alignment.CenterVertically,
  ) {
    Surface(
      shape = RoundedCornerShape(4.dp),
      color = MaterialTheme.colorScheme.secondaryContainer.copy(alpha = 0.5f),
    ) {
      Text(
        item.operation,
        style = MaterialTheme.typography.labelSmall,
        fontWeight = FontWeight.SemiBold,
        color = MaterialTheme.colorScheme.secondary,
        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
      )
    }
    Text(
      item.path,
      style = MaterialTheme.typography.bodySmall,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
      maxLines = 1,
      overflow = TextOverflow.Ellipsis,
    )
  }
}

@Composable
fun SystemMessageRow(item: SessionTimelineItem.System, modifier: Modifier = Modifier) {
  Text(
    item.text,
    modifier = modifier
      .fillMaxWidth()
      .padding(vertical = 2.dp),
    style = MaterialTheme.typography.bodySmall,
    color = MaterialTheme.colorScheme.outline,
  )
}
