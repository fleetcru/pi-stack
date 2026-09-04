package com.example.picompanion.ui.settings

import android.graphics.Color
import android.os.Bundle
import android.view.View
import android.widget.Button
import androidx.core.view.WindowCompat
import com.example.picompanion.R
import com.journeyapps.barcodescanner.CaptureActivity
import com.journeyapps.barcodescanner.DecoratedBarcodeView

/** Branded camera screen for scanning one trusted-device pairing QR code. */
class PairingScanActivity : CaptureActivity() {
  private lateinit var scanner: DecoratedBarcodeView
  private var torchOn = false

  override fun initializeContent(): DecoratedBarcodeView {
    setContentView(R.layout.activity_pairing_scan)
    return findViewById<DecoratedBarcodeView>(R.id.zxing_barcode_scanner).also {
      scanner = it
      // Replace ZXing's gray rectangle with the themed scan overlay.
      it.viewFinder.visibility = View.INVISIBLE
    }
  }

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    // Keep the header below the system bars. Transparent bars made the title
    // overlap the clock and cut off the first line on some phones.
    WindowCompat.setDecorFitsSystemWindows(window, true)
    window.statusBarColor = Color.BLACK
    window.navigationBarColor = Color.BLACK
    window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_VISIBLE
    findViewById<View>(R.id.pairing_scan_cancel).setOnClickListener { finish() }
    val torchButton = findViewById<Button>(R.id.pairing_scan_torch)
    torchButton.setOnClickListener {
      torchOn = !torchOn
      if (torchOn) scanner.setTorchOn() else scanner.setTorchOff()
      torchButton.text = if (torchOn) getString(R.string.pairing_scan_flash_off) else getString(R.string.pairing_scan_flash_on)
    }
  }

  override fun onPause() {
    if (::scanner.isInitialized && torchOn) {
      scanner.setTorchOff()
      torchOn = false
    }
    super.onPause()
  }
}
