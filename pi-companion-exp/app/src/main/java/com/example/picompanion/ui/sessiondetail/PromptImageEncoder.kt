package com.example.picompanion.ui.sessiondetail

import android.graphics.Bitmap
import java.util.Base64
import com.example.picompanion.data.api.PromptImage
import java.io.ByteArrayOutputStream

internal object PromptImageEncoder {
  const val JPEG_MIME_TYPE = "image/jpeg"

  fun encodeJpeg(bitmap: Bitmap, quality: Int = 85): PromptImage {
    val output = ByteArrayOutputStream()
    check(bitmap.compress(Bitmap.CompressFormat.JPEG, quality, output)) {
      "Bitmap could not be encoded as JPEG"
    }
    return fromJpegBytes(output.toByteArray())
  }

  internal fun fromJpegBytes(bytes: ByteArray): PromptImage {
    require(bytes.size >= 4 && bytes[0] == 0xFF.toByte() && bytes[1] == 0xD8.toByte() && bytes[bytes.lastIndex - 1] == 0xFF.toByte() && bytes.last() == 0xD9.toByte()) {
      "Encoded image is not a complete JPEG"
    }
    return PromptImage(
      base64 = Base64.getEncoder().encodeToString(bytes),
      mimeType = JPEG_MIME_TYPE,
    )
  }
}
