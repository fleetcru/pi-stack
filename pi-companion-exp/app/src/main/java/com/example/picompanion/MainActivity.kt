package com.example.picompanion

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.notifications.SessionNotificationManager
import com.example.picompanion.theme.PiCompanionTheme

class MainActivity : ComponentActivity() {
  private val notificationPermission = registerForActivityResult(
    ActivityResultContracts.RequestPermission(),
  ) { /* SessionNotificationManager checks the current grant before posting. */ }

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    AppModule.init(this)
    SessionNotificationManager.initialize(this)
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
      checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
    ) {
      notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    enableEdgeToEdge()
    setContent {
      var darkTheme by remember { mutableStateOf(true) }
      PiCompanionTheme(darkTheme = darkTheme) {
        Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
          MainNavigation(darkTheme = darkTheme, onDarkThemeChange = { darkTheme = it })
        }
      }
    }
  }
}
