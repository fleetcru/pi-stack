package com.example.picompanion.ui.settings

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.RectF
import android.util.AttributeSet
import android.view.View
import kotlin.math.min

/** Draws the same quiet rounded scan treatment used by the Companion UI. */
class PairingScanOverlayView @JvmOverloads constructor(
  context: Context,
  attrs: AttributeSet? = null,
) : View(context, attrs) {
  private val density = resources.displayMetrics.density
  private val cornerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
    color = 0xFFF4F2FA.toInt()
    style = Paint.Style.STROKE
    strokeWidth = 4f * density
    strokeCap = Paint.Cap.ROUND
    strokeJoin = Paint.Join.ROUND
  }
  private val laserPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
    color = 0xFFF4F2FA.toInt()
    style = Paint.Style.STROKE
    strokeWidth = 2f * density
    strokeCap = Paint.Cap.ROUND
  }
  override fun onDraw(canvas: Canvas) {
    super.onDraw(canvas)
    val size = min(width * 0.72f, height * 0.60f)
    val left = (width - size) / 2f
    val top = (height - size) / 2f - 8f * density
    val right = left + size
    val bottom = top + size
    val length = 58f * density
    val radius = 22f * density
    val frame = RectF(left, top, right, bottom)

    // Four open corners instead of the library's heavy gray rectangle.
    canvas.drawArc(RectF(left, top, left + radius * 2, top + radius * 2), 180f, 90f, false, cornerPaint)
    canvas.drawLine(left + radius, top, left + length, top, cornerPaint)
    canvas.drawLine(left, top + radius, left, top + length, cornerPaint)
    canvas.drawArc(RectF(right - radius * 2, top, right, top + radius * 2), 270f, 90f, false, cornerPaint)
    canvas.drawLine(right - radius, top, right - length, top, cornerPaint)
    canvas.drawLine(right, top + radius, right, top + length, cornerPaint)
    canvas.drawArc(RectF(left, bottom - radius * 2, left + radius * 2, bottom), 90f, 90f, false, cornerPaint)
    canvas.drawLine(left + radius, bottom, left + length, bottom, cornerPaint)
    canvas.drawLine(left, bottom - radius, left, bottom - length, cornerPaint)
    canvas.drawArc(RectF(right - radius * 2, bottom - radius * 2, right, bottom), 0f, 90f, false, cornerPaint)
    canvas.drawLine(right - radius, bottom, right - length, bottom, cornerPaint)
    canvas.drawLine(right, bottom - radius, right, bottom - length, cornerPaint)

    canvas.drawLine(left, top + size / 2f, right, top + size / 2f, laserPaint)
  }
}
