package com.example.picompanion.ui.settings

import android.content.Context
import android.util.AttributeSet
import android.view.View
import android.widget.FrameLayout
import kotlin.math.min

/** A scanner card that stays square on both short and tall phone screens. */
class PairingScanSquareLayout @JvmOverloads constructor(
  context: Context,
  attrs: AttributeSet? = null,
) : FrameLayout(context, attrs) {
  override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
    val width = MeasureSpec.getSize(widthMeasureSpec)
    val height = when (MeasureSpec.getMode(heightMeasureSpec)) {
      MeasureSpec.UNSPECIFIED -> width
      else -> min(width, MeasureSpec.getSize(heightMeasureSpec))
    }
    val size = min(width, height)
    val exactSize = MeasureSpec.makeMeasureSpec(size, MeasureSpec.EXACTLY)
    super.onMeasure(exactSize, exactSize)
    setMeasuredDimension(size, size)
  }
}
