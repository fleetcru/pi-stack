package com.example.picompanion.ui.settings

import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalFocusManager
import kotlinx.coroutines.delay
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp

@Composable
fun SettingsRow(
  label: String,
  value: String,
  modifier: Modifier = Modifier,
  onClick: (() -> Unit)? = null,
) {
  val rowModifier = if (onClick != null) {
    modifier.clickable(
      interactionSource = remember { MutableInteractionSource() },
      indication = null,
      onClick = onClick,
    )
  } else {
    modifier
  }

  Row(
    rowModifier
      .fillMaxWidth()
      .padding(vertical = 12.dp),
    horizontalArrangement = Arrangement.SpaceBetween,
    verticalAlignment = Alignment.CenterVertically,
  ) {
    Text(
      text = label,
      style = MaterialTheme.typography.bodyMedium,
      color = MaterialTheme.colorScheme.onSurface,
      modifier = Modifier.weight(1f),
    )
    Text(
      text = value,
      style = MaterialTheme.typography.bodyMedium,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
  }
}

@Composable
fun SettingsToggleRow(
  label: String,
  checked: Boolean,
  onCheckedChange: (Boolean) -> Unit,
  modifier: Modifier = Modifier,
  enabled: Boolean = true,
) {
  Row(
    modifier
      .fillMaxWidth()
      .padding(vertical = 9.dp),
    horizontalArrangement = Arrangement.SpaceBetween,
    verticalAlignment = Alignment.CenterVertically,
  ) {
    Text(
      text = label,
      style = MaterialTheme.typography.bodyMedium,
      color = if (enabled) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant,
      modifier = Modifier.weight(1f),
    )
    Switch(
      checked = checked,
      onCheckedChange = onCheckedChange,
      enabled = enabled,
      colors = SwitchDefaults.colors(
        checkedThumbColor = MaterialTheme.colorScheme.primary,
        checkedTrackColor = MaterialTheme.colorScheme.primaryContainer,
      ),
    )
  }
}

/**
 * Stacked editable field: label above, text input below.
 * Persists on every keystroke and clears focus on "Done"/"Next" IME action.
 */
@Composable
fun SettingsEditableRow(
  label: String,
  value: String,
  onValueChange: (String) -> Unit,
  modifier: Modifier = Modifier,
  masked: Boolean = false,
  placeholder: String = "",
  imeAction: ImeAction = ImeAction.Done,
  keyboardType: KeyboardType = KeyboardType.Text,
) {
  var editText by remember(value) { mutableStateOf(value) }
  val focusManager = LocalFocusManager.current

  Column(
    modifier
      .fillMaxWidth()
      .padding(vertical = 4.dp),
    verticalArrangement = Arrangement.spacedBy(4.dp),
  ) {
    Text(
      text = label,
      style = MaterialTheme.typography.labelMedium,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    OutlinedTextField(
      value = editText,
      onValueChange = { editText = it },
      modifier = Modifier.fillMaxWidth(),
      shape = RoundedCornerShape(12.dp),
      singleLine = true,
      textStyle = MaterialTheme.typography.bodyMedium,
      placeholder = {
        if (placeholder.isNotEmpty()) {
          Text(placeholder, style = MaterialTheme.typography.bodyMedium)
        }
      },
      visualTransformation = if (masked) PasswordVisualTransformation() else VisualTransformation.None,
      keyboardOptions = KeyboardOptions(
        imeAction = imeAction,
        keyboardType = keyboardType,
      ),
      keyboardActions = KeyboardActions(
        onDone = { focusManager.clearFocus() },
        onNext = { focusManager.clearFocus() },
      ),
      colors = OutlinedTextFieldDefaults.colors(
        focusedBorderColor = MaterialTheme.colorScheme.primary,
        unfocusedBorderColor = MaterialTheme.colorScheme.outline,
        cursorColor = MaterialTheme.colorScheme.primary,
      ),
    )
  }

  // Persist with debounce to avoid a DataStore write on every keystroke.
  LaunchedEffect(editText) {
    if (editText != value) {
      delay(300)
      onValueChange(editText)
    }
  }
}
