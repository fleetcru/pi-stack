package com.example.picompanion.ui.sessiondetail

import java.util.Base64
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class PromptImageEncoderTest {
  @Test
  fun jpegBytesKeepJpegMimeTypeAndMagicBytes() {
    val jpeg = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 0x01, 0x02, 0xFF.toByte(), 0xD9.toByte())

    val image = PromptImageEncoder.fromJpegBytes(jpeg)

    assertEquals("image/jpeg", image.mimeType)
    assertArrayEquals(jpeg, Base64.getDecoder().decode(image.base64))
  }

  @Test(expected = IllegalArgumentException::class)
  fun rejectsBytesWithoutJpegMarkers() {
    PromptImageEncoder.fromJpegBytes(byteArrayOf(1, 2, 3, 4))
  }
}
