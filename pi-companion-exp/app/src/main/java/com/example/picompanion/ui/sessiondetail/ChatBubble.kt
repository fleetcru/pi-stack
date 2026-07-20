package com.example.picompanion.ui.sessiondetail

import android.graphics.BitmapFactory
import android.net.Uri
import java.text.DateFormat
import java.util.Date
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ProvideTextStyle
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import com.halilibo.richtext.commonmark.Markdown
import com.halilibo.richtext.ui.material3.RichText as MarkdownRichText

@Composable
fun ChatBubble(
  author: String,
  text: String,
  time: String,
  isUser: Boolean,
  imageUris: List<Uri> = emptyList(),
  modifier: Modifier = Modifier,
) {
  val displayTime = formatChatTime(time)
  if (isUser) {
    Column(modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
      Text(
        text = author,
        style = MaterialTheme.typography.labelSmall,
        fontWeight = FontWeight.SemiBold,
        color = MaterialTheme.colorScheme.outline,
        modifier = Modifier.padding(horizontal = 4.dp, vertical = 2.dp),
      )
      Column(
        modifier = Modifier
          .widthIn(max = 292.dp)
          .background(MaterialTheme.colorScheme.onSurface, RoundedCornerShape(20.dp, 20.dp, 10.dp, 20.dp))
          .padding(horizontal = 14.dp, vertical = 10.dp),
      ) {
        if (imageUris.isNotEmpty()) {
          Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            imageUris.take(3).forEach { uri -> AttachmentThumbnail(uri) }
            if (imageUris.size > 3) Text("+${imageUris.size - 3}", color = MaterialTheme.colorScheme.surface)
          }
          if (text.isNotBlank()) Spacer(Modifier.height(8.dp))
        }
        if (text.isNotBlank()) Text(text = text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.surface)
        if (displayTime != null) Text(displayTime, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.surfaceVariant, modifier = Modifier.padding(top = 4.dp))
      }
    }
  } else {
    Column(modifier = modifier.fillMaxWidth()) {
      Surface(
        shape = RoundedCornerShape(20.dp),
        color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.35f),
        modifier = Modifier.fillMaxWidth(),
      ) {
        ProvideTextStyle(MaterialTheme.typography.bodyMedium) {
          MarkdownRichText(modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp)) { Markdown(text) }
        }
      }
      if (displayTime != null) Text(displayTime, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, modifier = Modifier.padding(start = 4.dp, top = 4.dp))
    }
  }
}

private fun formatChatTime(raw: String): String? {
  if (raw.isBlank()) return null
  if (raw == "now") return "Now"
  val epochMillis = raw.toLongOrNull() ?: return null
  return DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(epochMillis))
}

@Composable
private fun AttachmentThumbnail(uri: Uri) {
  val context = LocalContext.current
  // Decode bitmap off the main thread to avoid jank on large camera captures.
  val bitmap = produceState<android.graphics.Bitmap?>(null, uri) {
    value = withContext(Dispatchers.IO) {
      runCatching { context.contentResolver.openInputStream(uri)?.use(BitmapFactory::decodeStream) }.getOrNull()
    }
  }.value
  if (bitmap != null) {
    Image(
      bitmap = bitmap.asImageBitmap(),
      contentDescription = "Attached image",
      contentScale = ContentScale.Crop,
      modifier = Modifier.width(88.dp).height(68.dp).background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(12.dp)), 
    )
  } else {
    Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surfaceVariant, modifier = Modifier.width(88.dp).height(68.dp)) {
      Text("Image", modifier = Modifier.padding(8.dp), style = MaterialTheme.typography.labelSmall)
    }
  }
}
