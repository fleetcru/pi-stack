package com.example.picompanion.ui.settings

import android.os.Bundle
import android.widget.Button
import com.example.picompanion.R
import com.journeyapps.barcodescanner.CaptureActivity
import com.journeyapps.barcodescanner.DecoratedBarcodeView

/** Branded camera screen for scanning one trusted-device pairing QR code. */
class PairingScanActivity : CaptureActivity() {
  private lateinit var scanner: DecoratedBarcodeView
  private var torchOn = false

  override fun initializeContent(): DecoratedBarcodeView {
    setContentView(R.layout.activity_pairing_scan)
    return findViewById<DecoratedBarcodeView>(R.id.zxing_barcode_scanner).also { scanner = it }
  }

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    findViewById<Button>(R.id.pairing_scan_cancel).setOnClickListener { finish() }
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
