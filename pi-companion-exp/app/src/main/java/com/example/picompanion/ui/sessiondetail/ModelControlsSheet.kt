package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

// Intentionally dark-themed: this sheet always renders with a fixed dark
// background regardless of the app theme, similar to an OLED-dark UI.
// The high-contrast palette ensures readability on all device modes.
private val ModelSheetBlack = Color(0xFF151515)
private val ModelSheetCard = Color(0xFF252525)
private val ModelSheetLine = Color(0xFF414141)
private val ModelSheetText = Color(0xFFF4F4F4)
private val ModelSheetMuted = Color(0xFFA9A9A9)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ModelControlsSheet(
  controls: ModelControls,
  onDismiss: () -> Unit,
  onSelectModel: (String, String) -> Unit,
  onSelectThinking: (String) -> Unit,
) {
  val providers = controls.models.map { it.provider }.distinct()
  var provider by remember(controls.selectedProvider, providers) { mutableStateOf(controls.selectedProvider ?: providers.firstOrNull()) }
  var providerMenuOpen by remember { mutableStateOf(false) }
  val visibleModels = controls.models.filter { it.provider == provider }

  ModalBottomSheet(
    onDismissRequest = onDismiss,
    sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    containerColor = ModelSheetBlack,
    contentColor = ModelSheetText,
  ) {
    Column(
      modifier = Modifier
        .fillMaxWidth()
        .heightIn(max = 620.dp)
        .verticalScroll(rememberScrollState())
        .background(ModelSheetBlack)
        .padding(start = 20.dp, end = 20.dp, bottom = 28.dp),
      verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
      Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Column {
          Text("Model & effort", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
          Text("Choose the agent configuration", style = MaterialTheme.typography.bodySmall, color = ModelSheetMuted)
        }
        TextButton(onClick = onDismiss) { Text("Done", color = ModelSheetText) }
      }

      SheetLabel("PROVIDER")
      Surface(shape = RoundedCornerShape(12.dp), color = ModelSheetCard, modifier = Modifier.fillMaxWidth()) {
        TextButton(onClick = { providerMenuOpen = true }, modifier = Modifier.fillMaxWidth(), contentPadding = PaddingValues(horizontal = 14.dp, vertical = 12.dp)) {
          Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(provider ?: "No providers available", color = ModelSheetText)
            Text("Change", color = ModelSheetMuted, style = MaterialTheme.typography.labelMedium)
          }
        }
        DropdownMenu(expanded = providerMenuOpen, onDismissRequest = { providerMenuOpen = false }) {
          providers.forEach { value -> DropdownMenuItem(text = { Text(value) }, onClick = { provider = value; providerMenuOpen = false }) }
        }
      }

      SheetLabel("MODELS")
      if (visibleModels.isEmpty()) {
        Text("No models reported by this provider.", style = MaterialTheme.typography.bodySmall, color = ModelSheetMuted)
      } else {
        visibleModels.forEach { model ->
          val selected = controls.selectedProvider == model.provider && controls.selectedModelId == model.id
          Surface(
            onClick = { onSelectModel(model.provider, model.id) },
            shape = RoundedCornerShape(12.dp),
            color = if (selected) Color(0xFFF0F0F0) else ModelSheetCard,
            modifier = Modifier.fillMaxWidth(),
          ) {
            Column(Modifier.padding(horizontal = 14.dp, vertical = 11.dp)) {
              Text(model.name, fontWeight = FontWeight.SemiBold, color = if (selected) ModelSheetBlack else ModelSheetText)
              Text(model.id, style = MaterialTheme.typography.labelSmall, color = if (selected) Color(0xFF555555) else ModelSheetMuted)
            }
          }
        }
      }

      HorizontalDivider(color = ModelSheetLine)
      SheetLabel("THINKING / EFFORT")
      Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf("off", "low", "medium").forEach { level -> EffortChip(level, controls.thinkingLevel, onSelectThinking) }
      }
      Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf("high", "xhigh", "max").forEach { level -> EffortChip(level, controls.thinkingLevel, onSelectThinking) }
      }
    }
  }
}

@Composable
private fun SheetLabel(text: String) = Text(text, style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold, color = ModelSheetMuted)

@Composable
private fun EffortChip(level: String, selectedLevel: String?, onSelect: (String) -> Unit) {
  FilterChip(
    selected = selectedLevel == level,
    onClick = { onSelect(level) },
    label = { Text(level) },
  )
}
