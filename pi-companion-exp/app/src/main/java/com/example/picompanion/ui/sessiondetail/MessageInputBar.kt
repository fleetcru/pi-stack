package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowUpward
import androidx.compose.material.icons.rounded.AttachFile
import androidx.compose.material.icons.rounded.CameraAlt
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
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
  attachmentCount: Int = 0,
  attachmentNames: List<String> = emptyList(),
  onRemoveAttachment: ((Int) -> Unit)? = null,
) {
  var text by remember { mutableStateOf("") }
  val aborting = agentWorking && onAbort != null
  val canSend = text.isNotBlank() && !sending && !aborting

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
      if (attachmentNames.isNotEmpty()) {
        Row(
          modifier = Modifier.fillMaxWidth().padding(top = 4.dp, bottom = 2.dp),
          horizontalArrangement = Arrangement.spacedBy(6.dp),
          verticalAlignment = Alignment.CenterVertically,
        ) {
          attachmentNames.take(3).forEachIndexed { index, name ->
            Surface(shape = RoundedCornerShape(10.dp), color = MaterialTheme.colorScheme.secondaryContainer) {
              Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(start = 8.dp, end = 2.dp, top = 3.dp, bottom = 3.dp)) {
                Text(name, maxLines = 1, style = MaterialTheme.typography.labelSmall, modifier = Modifier.width(130.dp))
                if (onRemoveAttachment != null) IconButton(onClick = { onRemoveAttachment(index) }, modifier = Modifier.size(24.dp)) {
                  Icon(Icons.Rounded.Close, contentDescription = "Remove $name", modifier = Modifier.size(14.dp))
                }
              }
            }
          }
          if (attachmentNames.size > 3) Text("+${attachmentNames.size - 3}", style = MaterialTheme.typography.labelSmall)
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
        ),
      )

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
