package com.example.picompanion.ui.sessiondetail

import android.graphics.BitmapFactory
import android.net.Uri
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowUpward
import androidx.compose.material.icons.rounded.AttachFile
import androidx.compose.material.icons.rounded.CameraAlt
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

@Composable
fun MessageInputBar(
  onSend: (String) -> Unit,
  modifier: Modifier = Modifier,
  sending: Boolean = false,
  agentWorking: Boolean = false,
  status: String? = null,
  onAbort: (() -> Unit)? = null,
  onSteer: ((String) -> Unit)? = null,
  onPickImage: (() -> Unit)? = null,
  onTakePhoto: (() -> Unit)? = null,
  attachmentUris: List<Uri> = emptyList(),
  onRemoveAttachment: ((Int) -> Unit)? = null,
) {
  var text by remember { mutableStateOf("") }
  val slashCommands = remember(text) {
    listOf("/help" to "Show Pi help", "/clear" to "Clear conversation", "/compact" to "Compact conversation", "/model" to "Change model", "/settings" to "Open settings", "/reload" to "Reload extensions", "/session" to "Show session information", "/tps" to "Show token and speed totals")
      .filter { text.trim().split(Regex("\\s+"), limit = 2).firstOrNull()?.let { token -> token.startsWith("/") && it.first.startsWith(token, ignoreCase = true) } == true }
  }
  val aborting = agentWorking && onAbort != null
  val canSend = (text.isNotBlank() || attachmentUris.isNotEmpty()) && !sending && !aborting

  Surface(
    modifier = modifier
      .fillMaxWidth()
      .padding(horizontal = 12.dp, vertical = 8.dp),
    shape = RoundedCornerShape(20.dp),
    color = MaterialTheme.colorScheme.surfaceVariant,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
  ) {
    Column(
      modifier = Modifier.padding(start = 12.dp, end = 8.dp, top = 4.dp, bottom = 6.dp),
      verticalArrangement = Arrangement.spacedBy(0.dp),
    ) {
      if (attachmentUris.isNotEmpty()) {
        Row(
          modifier = Modifier.fillMaxWidth().padding(top = 4.dp, bottom = 2.dp),
          horizontalArrangement = Arrangement.spacedBy(6.dp),
          verticalAlignment = Alignment.CenterVertically,
        ) {
          attachmentUris.take(3).forEachIndexed { index, uri ->
            Box {
              AttachmentThumbnailPreview(uri)
              if (onRemoveAttachment != null) {
                IconButton(
                  onClick = { onRemoveAttachment(index) },
                  modifier = Modifier.align(Alignment.TopEnd).size(24.dp),
                ) {
                  Icon(Icons.Rounded.Close, contentDescription = "Remove image", modifier = Modifier.size(14.dp))
                }
              }
            }
          }
          if (attachmentUris.size > 3) Text("+${attachmentUris.size - 3}", style = MaterialTheme.typography.labelSmall)
        }
      }
      TextField(
        value = text,
        onValueChange = { text = it },
        modifier = Modifier.fillMaxWidth(),
        enabled = !sending,
        placeholder = {
          Text(
            text = status ?: if (sending) "Waiting for response…" else "Ask Pi Companion",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        },
        minLines = 1,
        maxLines = 5,
        colors = TextFieldDefaults.colors(
          focusedContainerColor = Color.Transparent,
          unfocusedContainerColor = Color.Transparent,
          disabledContainerColor = Color.Transparent,
          focusedIndicatorColor = Color.Transparent,
          unfocusedIndicatorColor = Color.Transparent,
          disabledIndicatorColor = Color.Transparent,
        ),
      )
      if (slashCommands.isNotEmpty()) {
        Column(modifier = Modifier.fillMaxWidth().padding(bottom = 4.dp)) {
          slashCommands.forEach { (command, description) ->
            Row(modifier = Modifier.fillMaxWidth().clickable { text = "$command " }.padding(horizontal = 8.dp, vertical = 6.dp)) {
              Text(command, style = MaterialTheme.typography.labelLarge)
              Text("  $description", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
          }
          Text("Tap a command to complete it", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 8.dp))
        }
      }

      Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
      ) {
        // Utility controls stay on the left; the primary Send/Stop action is
        // the only control on the right.
        Row(verticalAlignment = Alignment.CenterVertically) {
          if (!aborting) {
          if (onPickImage != null) IconButton(onClick = onPickImage, modifier = Modifier.size(34.dp)) {
            Icon(Icons.Rounded.AttachFile, contentDescription = "Attach image", tint = MaterialTheme.colorScheme.onSurfaceVariant)
          }
          if (onTakePhoto != null) IconButton(onClick = onTakePhoto, modifier = Modifier.size(34.dp)) {
            Icon(Icons.Rounded.CameraAlt, contentDescription = "Take photo", tint = MaterialTheme.colorScheme.onSurfaceVariant)
          }
          }
          // Steer button: appears when agent is working and there's text to steer with.
          // Steer redirects the agent without starting a new turn.
          if (onSteer != null && agentWorking && text.isNotBlank() && !sending) {
            IconButton(onClick = { onSteer(text.trim()); text = "" }, modifier = Modifier.size(34.dp)) {
              Icon(Icons.Rounded.Tune, contentDescription = "Steer", tint = MaterialTheme.colorScheme.tertiary)
            }
          }
        }

        Box(
          modifier = Modifier
            .size(36.dp)
            .clip(CircleShape)
            .background(
              when {
                aborting -> MaterialTheme.colorScheme.error
                canSend -> MaterialTheme.colorScheme.primary
                else -> MaterialTheme.colorScheme.surface
              },
            ),
          contentAlignment = Alignment.Center,
        ) {
          IconButton(
            onClick = {
              if (aborting) onAbort()
              else if (canSend) {
                onSend(text.trim())
                text = ""
              }
            },
            enabled = aborting || canSend,
          ) {
            Icon(
              imageVector = if (aborting) Icons.Rounded.Stop else Icons.Rounded.ArrowUpward,
              contentDescription = if (aborting) "Abort agent" else "Send",
              tint = when {
                aborting -> MaterialTheme.colorScheme.onError
                canSend -> MaterialTheme.colorScheme.onPrimary
                else -> MaterialTheme.colorScheme.outline
              },
            )
          }
        }
      }
    }
  }
}

@Composable
private fun AttachmentThumbnailPreview(uri: Uri) {
  val context = LocalContext.current
  val bitmap = produceState<android.graphics.Bitmap?>(null, uri) {
    value = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
      runCatching {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
        val maxPx = (96 * context.resources.displayMetrics.density).toInt().coerceAtLeast(64)
        val sample = maxOf(1, maxOf(bounds.outWidth, bounds.outHeight) / maxPx)
        val options = BitmapFactory.Options().apply { inSampleSize = sample }
        context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) }
      }.getOrNull()
    }
  }.value
  if (bitmap != null) {
    Image(
      bitmap = bitmap.asImageBitmap(),
      contentDescription = "Attached image",
      contentScale = ContentScale.Crop,
      modifier = Modifier.size(width = 72.dp, height = 56.dp).clip(RoundedCornerShape(10.dp)),
    )
  } else {
    Surface(
      shape = RoundedCornerShape(10.dp),
      color = MaterialTheme.colorScheme.secondaryContainer,
      modifier = Modifier.size(width = 72.dp, height = 56.dp),
    ) { Text("Image", modifier = Modifier.padding(8.dp), style = MaterialTheme.typography.labelSmall) }
  }
}
