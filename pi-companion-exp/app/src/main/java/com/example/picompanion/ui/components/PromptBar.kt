package com.example.picompanion.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
fun PromptBar(value: String, onValueChange: (String) -> Unit, onSend: () -> Unit, modifier: Modifier = Modifier) {
  Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
    OutlinedTextField(value = value, onValueChange = onValueChange, modifier = Modifier.weight(1f), placeholder = { Text("Prompt or steer…") })
    Button(onClick = onSend) { Text("Send") }
  }
}
