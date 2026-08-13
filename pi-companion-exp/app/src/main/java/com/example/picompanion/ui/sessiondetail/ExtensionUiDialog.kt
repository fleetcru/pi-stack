package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.example.picompanion.data.model.ExtensionUiMethod
import com.example.picompanion.data.model.ExtensionUiOption
import com.example.picompanion.data.model.ExtensionUiRequest

/** Structured answer mapped onto the `/ui-response` body by the ViewModel. */
data class ExtensionUiAnswer(
  val value: String? = null,
  val confirmed: Boolean? = null,
  val selections: List<String>? = null,
  val comment: String? = null,
  val responseKind: String? = null,
)

/** Minimal, bounded choice card for Pi extension UI requests. */
@Composable
fun ExtensionUiDialog(
  request: ExtensionUiRequest,
  submitError: String?,
  submitting: Boolean,
  onConfirm: (ExtensionUiAnswer) -> Unit,
  onCancel: () -> Unit,
  onDismissLocal: () -> Unit,
) {
  var selected by remember(request.id) { mutableStateOf<String?>(null) }
  var selectedMulti by remember(request.id) { mutableStateOf<Set<String>>(emptySet()) }
  var freeform by remember(request.id) { mutableStateOf("") }
  var comment by remember(request.id) { mutableStateOf("") }
  var showFreeform by remember(request.id) {
    mutableStateOf(request.method == ExtensionUiMethod.ASK_USER && request.options.isEmpty())
  }
  var showComment by remember(request.id) { mutableStateOf(false) }
  var textValue by remember(request.id) { mutableStateOf(request.prefill ?: "") }

  val canSubmit = when (request.method) {
    ExtensionUiMethod.ASK_USER -> freeform.isNotBlank() || selected != null || selectedMulti.isNotEmpty()
    ExtensionUiMethod.SELECT -> selected != null
    else -> true
  }
  val submitLabel = when (request.method) {
    ExtensionUiMethod.CONFIRM -> "Yes"
    ExtensionUiMethod.INPUT,
    ExtensionUiMethod.EDITOR,
    ExtensionUiMethod.UNKNOWN,
    -> "Send"
    else -> "Submit"
  }

  Dialog(
    onDismissRequest = { if (!submitting) onCancel() },
    properties = DialogProperties(
      dismissOnBackPress = !submitting,
      dismissOnClickOutside = false,
      usePlatformDefaultWidth = false,
      decorFitsSystemWindows = false,
    ),
  ) {
    BoxWithConstraints(
      modifier = Modifier
        .fillMaxSize()
        .imePadding()
        .padding(horizontal = 24.dp, vertical = 32.dp),
      contentAlignment = Alignment.Center,
    ) {
      Surface(
        modifier = Modifier
          .fillMaxWidth()
          .widthIn(max = 380.dp)
          .heightIn(max = maxHeight * 0.74f)
          .testTag("extension-ui-dialog"),
        shape = RoundedCornerShape(22.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        tonalElevation = 6.dp,
        shadowElevation = 10.dp,
      ) {
        Column {
          Column(
            modifier = Modifier
              .weight(1f, fill = false)
              .verticalScroll(rememberScrollState())
              .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
          ) {
            Text(
              text = request.displayText,
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
              color = MaterialTheme.colorScheme.onSurface,
            )
            if (!request.context.isNullOrBlank()) {
              Text(
                text = request.context,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
              )
            }

            when (request.method) {
              ExtensionUiMethod.ASK_USER -> {
                if (request.options.isNotEmpty()) {
                  ChoiceHint(if (request.isMultiSelect) "Choose any that apply" else "Choose one")
                  request.options.forEach { option ->
                    ChoiceCard(
                      option = option,
                      selected = if (request.isMultiSelect) {
                        option.title in selectedMulti
                      } else {
                        option.title == selected
                      },
                      multiSelect = request.isMultiSelect,
                      enabled = !submitting,
                      onToggle = { checked ->
                        freeform = ""
                        showFreeform = false
                        if (request.isMultiSelect) {
                          selectedMulti = if (checked) {
                            selectedMulti + option.title
                          } else {
                            selectedMulti - option.title
                          }
                        } else {
                          selected = option.title
                        }
                      },
                    )
                  }
                }

                if (request.supportsFreeform || request.options.isEmpty()) {
                  if (request.options.isNotEmpty()) OtherAnswerCard(
                    selected = showFreeform,
                    enabled = !submitting,
                    onClick = {
                      selected = null
                      selectedMulti = emptySet()
                      showFreeform = true
                    },
                  )
                  if (showFreeform) {
                    OutlinedTextField(
                      value = freeform,
                      onValueChange = { freeform = it },
                      enabled = !submitting,
                      label = { Text("Your answer") },
                      placeholder = { Text(request.placeholder ?: "Type another option") },
                      singleLine = true,
                      modifier = Modifier
                        .fillMaxWidth()
                        .testTag("extension-ui-other-input"),
                      shape = RoundedCornerShape(14.dp),
                    )
                  }
                }

                if (request.supportsComment && !showFreeform) {
                  if (showComment) {
                    OutlinedTextField(
                      value = comment,
                      onValueChange = { comment = it },
                      enabled = !submitting,
                      label = { Text("Note (optional)") },
                      modifier = Modifier.fillMaxWidth(),
                      shape = RoundedCornerShape(14.dp),
                      minLines = 2,
                    )
                  } else {
                    TextButton(
                      onClick = { showComment = true },
                      enabled = !submitting,
                    ) {
                      Text("Add a note")
                    }
                  }
                }
              }

              ExtensionUiMethod.SELECT -> {
                ChoiceHint("Choose one")
                request.options.forEach { option ->
                  ChoiceCard(
                    option = option,
                    selected = option.title == selected,
                    multiSelect = false,
                    enabled = !submitting,
                    onToggle = { selected = option.title },
                  )
                }
              }

              ExtensionUiMethod.CONFIRM -> Unit
              ExtensionUiMethod.INPUT -> ResponseField(
                value = textValue,
                onValueChange = { textValue = it },
                label = request.placeholder ?: "Response",
                enabled = !submitting,
                singleLine = true,
              )
              ExtensionUiMethod.EDITOR -> ResponseField(
                value = textValue,
                onValueChange = { textValue = it },
                label = request.placeholder ?: "Response",
                enabled = !submitting,
                singleLine = false,
              )
              ExtensionUiMethod.UNKNOWN -> ResponseField(
                value = textValue,
                onValueChange = { textValue = it },
                label = request.placeholder ?: "Response",
                enabled = !submitting,
                singleLine = false,
              )
            }

            if (submitError != null) {
              Surface(
                shape = RoundedCornerShape(12.dp),
                color = MaterialTheme.colorScheme.errorContainer,
              ) {
                Text(
                  text = submitError,
                  modifier = Modifier.padding(horizontal = 12.dp, vertical = 9.dp),
                  style = MaterialTheme.typography.bodySmall,
                  color = MaterialTheme.colorScheme.onErrorContainer,
                )
              }
            }
          }

          HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
          Row(
            modifier = Modifier
              .fillMaxWidth()
              .padding(horizontal = 10.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.End,
            verticalAlignment = Alignment.CenterVertically,
          ) {
            if (submitError != null) {
              TextButton(onClick = onDismissLocal, enabled = !submitting) {
                Text("Dismiss")
              }
            }
            TextButton(
              onClick = {
                if (request.method == ExtensionUiMethod.CONFIRM) {
                  onConfirm(ExtensionUiAnswer(confirmed = false))
                } else {
                  onCancel()
                }
              },
              enabled = !submitting,
            ) {
              Text(if (request.method == ExtensionUiMethod.CONFIRM) "No" else "Cancel")
            }
            Button(
              onClick = {
                onConfirm(
                  when (request.method) {
                    ExtensionUiMethod.ASK_USER -> if (freeform.isNotBlank()) {
                      ExtensionUiAnswer(
                        value = freeform.trim(),
                        comment = comment.ifBlank { null },
                        responseKind = "freeform",
                      )
                    } else {
                      ExtensionUiAnswer(
                        selections = if (request.isMultiSelect) {
                          selectedMulti.toList()
                        } else {
                          listOfNotNull(selected)
                        },
                        comment = comment.ifBlank { null },
                        responseKind = "selection",
                      )
                    }
                    ExtensionUiMethod.SELECT -> ExtensionUiAnswer(value = selected)
                    ExtensionUiMethod.CONFIRM -> ExtensionUiAnswer(confirmed = true)
                    ExtensionUiMethod.INPUT,
                    ExtensionUiMethod.EDITOR,
                    ExtensionUiMethod.UNKNOWN,
                    -> ExtensionUiAnswer(value = textValue)
                  },
                )
              },
              enabled = canSubmit && !submitting,
              modifier = Modifier.testTag("extension-ui-submit"),
            ) {
              if (submitting) {
                CircularProgressIndicator(
                  modifier = Modifier.size(18.dp),
                  strokeWidth = 2.dp,
                  color = MaterialTheme.colorScheme.onPrimary,
                )
              } else {
                Text(submitLabel)
              }
            }
          }
        }
      }
    }
  }
}

@Composable
private fun ChoiceHint(text: String) {
  Text(
    text = text,
    style = MaterialTheme.typography.labelMedium,
    color = MaterialTheme.colorScheme.onSurfaceVariant,
  )
}

@Composable
private fun ChoiceCard(
  option: ExtensionUiOption,
  selected: Boolean,
  multiSelect: Boolean,
  enabled: Boolean,
  onToggle: (Boolean) -> Unit,
) {
  val containerColor = if (selected) {
    MaterialTheme.colorScheme.secondaryContainer
  } else {
    MaterialTheme.colorScheme.surfaceContainerLow
  }
  val borderColor = if (selected) {
    MaterialTheme.colorScheme.primary
  } else {
    MaterialTheme.colorScheme.outlineVariant
  }

  Surface(
    modifier = Modifier
      .fillMaxWidth()
      .toggleable(
        value = selected,
        enabled = enabled,
        role = if (multiSelect) Role.Checkbox else Role.RadioButton,
        onValueChange = { checked -> onToggle(if (multiSelect) checked else true) },
      )
      .testTag("extension-ui-option-${option.title}"),
    shape = RoundedCornerShape(14.dp),
    color = containerColor,
    border = BorderStroke(if (selected) 2.dp else 1.dp, borderColor),
  ) {
    Row(
      modifier = Modifier
        .fillMaxWidth()
        .heightIn(min = 52.dp)
        .padding(horizontal = 14.dp, vertical = 9.dp),
      verticalAlignment = Alignment.CenterVertically,
      horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
      Column(
        modifier = Modifier.weight(1f),
        verticalArrangement = Arrangement.spacedBy(1.dp),
      ) {
        Text(
          text = option.title,
          style = MaterialTheme.typography.bodyMedium,
          fontWeight = FontWeight.SemiBold,
          color = MaterialTheme.colorScheme.onSurface,
        )
        if (!option.description.isNullOrBlank()) {
          Text(
            text = option.description,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }
      }
      SelectionMark(selected = selected, multiSelect = multiSelect)
    }
  }
}

@Composable
private fun OtherAnswerCard(
  selected: Boolean,
  enabled: Boolean,
  onClick: () -> Unit,
) {
  Surface(
    modifier = Modifier
      .fillMaxWidth()
      .toggleable(
        value = selected,
        enabled = enabled,
        role = Role.RadioButton,
        onValueChange = { onClick() },
      )
      .testTag("extension-ui-other"),
    shape = RoundedCornerShape(14.dp),
    color = if (selected) {
      MaterialTheme.colorScheme.secondaryContainer
    } else {
      MaterialTheme.colorScheme.surfaceContainerLow
    },
    border = BorderStroke(
      if (selected) 2.dp else 1.dp,
      if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant,
    ),
  ) {
    Row(
      modifier = Modifier
        .fillMaxWidth()
        .heightIn(min = 52.dp)
        .padding(horizontal = 14.dp, vertical = 9.dp),
      verticalAlignment = Alignment.CenterVertically,
    ) {
      Text(
        text = "Other",
        modifier = Modifier.weight(1f),
        style = MaterialTheme.typography.bodyMedium,
        fontWeight = FontWeight.SemiBold,
      )
      SelectionMark(selected = selected, multiSelect = false)
    }
  }
}

@Composable
private fun SelectionMark(selected: Boolean, multiSelect: Boolean) {
  Surface(
    modifier = Modifier.size(24.dp),
    shape = if (multiSelect) RoundedCornerShape(7.dp) else CircleShape,
    color = if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surface,
    border = BorderStroke(
      1.dp,
      if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline,
    ),
  ) {
    if (selected) {
      Row(
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
      ) {
        Text(
          text = "✓",
          style = MaterialTheme.typography.labelMedium,
          fontWeight = FontWeight.Bold,
          color = MaterialTheme.colorScheme.onPrimary,
        )
      }
    }
  }
}

@Composable
private fun ResponseField(
  value: String,
  onValueChange: (String) -> Unit,
  label: String,
  enabled: Boolean,
  singleLine: Boolean,
) {
  OutlinedTextField(
    value = value,
    onValueChange = onValueChange,
    enabled = enabled,
    label = { Text(label) },
    singleLine = singleLine,
    minLines = if (singleLine) 1 else 4,
    modifier = Modifier.fillMaxWidth(),
    shape = RoundedCornerShape(14.dp),
  )
}
