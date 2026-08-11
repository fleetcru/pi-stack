package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.example.picompanion.data.model.ExtensionUiMethod
import com.example.picompanion.data.model.ExtensionUiOption
import com.example.picompanion.data.model.ExtensionUiRequest

/**
 * Structured answer produced by the dialog, mapped onto the `/ui-response` body
 * (id is added by the ViewModel from the pending request).
 */
data class ExtensionUiAnswer(
  val value: String? = null,
  val confirmed: Boolean? = null,
  val selections: List<String>? = null,
  val comment: String? = null,
  val responseKind: String? = null,
)

/**
 * Dialog for structured Pi extension UI requests.
 *
 * Renders per method:
 * - [ExtensionUiMethod.ASK_USER]: single/multi select ([ExtensionUiRequest.allowMultiple]),
 *   optional freeform ([ExtensionUiRequest.allowFreeform]) and comment
 *   ([ExtensionUiRequest.allowComment]). A non-blank freeform answer wins over
 *   selections (freeform → `value` + `responseKind: "freeform"`); otherwise
 *   selections + optional comment are sent.
 * - [ExtensionUiMethod.SELECT]: single-choice list → `value`.
 * - [ExtensionUiMethod.CONFIRM]: message with Confirm/No → `confirmed`.
 * - [ExtensionUiMethod.INPUT]: single-line field → `value` (placeholder).
 * - [ExtensionUiMethod.EDITOR]: multi-line field → `value` (prefill).
 *
 * A submit failure is surfaced inside the dialog while the request stays open
 * so the user can retry.
 */
@Composable
fun ExtensionUiDialog(
  request: ExtensionUiRequest,
  submitError: String?,
  submitting: Boolean,
  onConfirm: (ExtensionUiAnswer) -> Unit,
  onCancel: () -> Unit,
  onDismissLocal: () -> Unit,
) {
  // State is keyed to the request id so a new request starts fresh.
  var selected by remember(request.id) { mutableStateOf<String?>(null) }
  var selectedMulti by remember(request.id) { mutableStateOf<Set<String>>(emptySet()) }
  var freeform by remember(request.id) { mutableStateOf("") }
  var comment by remember(request.id) { mutableStateOf("") }
  var textValue by remember(request.id) { mutableStateOf(request.prefill ?: "") }

  val canConfirm = when (request.method) {
    ExtensionUiMethod.ASK_USER -> selectedMulti.isNotEmpty() || selected != null || freeform.isNotBlank()
    ExtensionUiMethod.SELECT -> selected != null
    ExtensionUiMethod.CONFIRM,
    ExtensionUiMethod.INPUT,
    ExtensionUiMethod.EDITOR,
    ExtensionUiMethod.UNKNOWN,
    -> true
  }

  AlertDialog(
    onDismissRequest = onCancel,
    title = { Text(request.displayTitle) },
    text = {
      Column(
        modifier = Modifier
          .heightIn(max = 420.dp)
          .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(8.dp),
      ) {
        Text(request.displayText)
        if (!request.context.isNullOrBlank()) {
          Text(
            request.context,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }
        when (request.method) {
          ExtensionUiMethod.ASK_USER -> {
            if (request.options.isNotEmpty()) {
              Text(
                if (request.isMultiSelect) "Select all that apply" else "Select one",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
              )
              request.options.forEach { option ->
                OptionRow(
                  option = option,
                  selected = if (request.isMultiSelect) option.title in selectedMulti else option.title == selected,
                  multiSelect = request.isMultiSelect,
                  onToggle = { checked ->
                    freeform = ""
                    if (request.isMultiSelect) {
                      selectedMulti = if (checked) selectedMulti + option.title else selectedMulti - option.title
                    } else {
                      selected = if (checked) option.title else null
                    }
                  },
                )
              }
            }
            if (request.supportsFreeform) {
              OutlinedTextField(
                value = freeform,
                onValueChange = {
                  freeform = it
                  if (it.isNotBlank()) {
                    selected = null
                    selectedMulti = emptySet()
                  }
                },
                label = { Text("Custom answer") },
                placeholder = { Text(request.placeholder ?: "Type your own answer") },
                modifier = Modifier.fillMaxWidth(),
              )
            }
            if (request.supportsComment) {
              OutlinedTextField(
                value = comment,
                onValueChange = { comment = it },
                label = { Text("Comment (optional)") },
                modifier = Modifier.fillMaxWidth(),
              )
            }
          }
          ExtensionUiMethod.SELECT -> {
            request.options.forEach { option ->
              OptionRow(
                option = option,
                selected = option.title == selected,
                multiSelect = false,
                onToggle = { checked -> selected = if (checked) option.title else null },
              )
            }
          }
          ExtensionUiMethod.CONFIRM -> Unit // message already shown; buttons below
          ExtensionUiMethod.INPUT -> {
            OutlinedTextField(
              value = textValue,
              onValueChange = { textValue = it },
              label = { Text(request.placeholder ?: "Response") },
              singleLine = true,
              modifier = Modifier.fillMaxWidth(),
            )
          }
          ExtensionUiMethod.EDITOR -> {
            OutlinedTextField(
              value = textValue,
              onValueChange = { textValue = it },
              label = { Text(request.placeholder ?: "Response") },
              minLines = 4,
              modifier = Modifier.fillMaxWidth(),
            )
          }
          ExtensionUiMethod.UNKNOWN -> {
            OutlinedTextField(
              value = textValue,
              onValueChange = { textValue = it },
              label = { Text(request.placeholder ?: "Response") },
              modifier = Modifier.fillMaxWidth(),
            )
          }
        }
        HorizontalDivider()
        Text(
          "Request ID: ${request.id}. Only respond if you expected this extension prompt.",
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (submitError != null) {
          Text(
            submitError,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.error,
          )
          Text(
            "The request is still open — check your connection and try again.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.error,
          )
        }
      }
    },
    confirmButton = {
      TextButton(
        onClick = {
          onConfirm(
            when (request.method) {
              ExtensionUiMethod.ASK_USER ->
                if (freeform.isNotBlank()) {
                  ExtensionUiAnswer(value = freeform.trim(), responseKind = "freeform")
                } else {
                  ExtensionUiAnswer(
                    selections = if (request.isMultiSelect) selectedMulti.toList() else listOfNotNull(selected),
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
            }
          )
        },
        enabled = canConfirm && !submitting,
      ) { Text("Confirm") }
    },
    dismissButton = {
      Row {
        if (submitError != null) {
          TextButton(onClick = onDismissLocal, enabled = !submitting) { Text("Dismiss") }
        }
        if (request.method == ExtensionUiMethod.CONFIRM) {
          TextButton(
            onClick = { onConfirm(ExtensionUiAnswer(confirmed = false)) },
            enabled = !submitting,
          ) { Text("No") }
        } else {
          TextButton(onClick = onCancel, enabled = !submitting) { Text("Cancel") }
        }
      }
    },
  )
}

@Composable
private fun OptionRow(
  option: ExtensionUiOption,
  selected: Boolean,
  multiSelect: Boolean,
  onToggle: (Boolean) -> Unit,
) {
  Row(
    modifier = Modifier
      .fillMaxWidth()
      .selectable(
        selected = selected,
        onClick = { onToggle(!selected) },
        role = if (multiSelect) Role.Checkbox else Role.RadioButton,
      )
      .padding(vertical = 4.dp),
    verticalAlignment = Alignment.CenterVertically,
  ) {
    if (multiSelect) {
      Checkbox(checked = selected, onCheckedChange = null)
    } else {
      RadioButton(selected = selected, onClick = null)
    }
    Column(modifier = Modifier.padding(start = 8.dp)) {
      Text(
        option.title,
        style = MaterialTheme.typography.bodyMedium,
        fontWeight = FontWeight.Medium,
      )
      if (!option.description.isNullOrBlank()) {
        Text(
          option.description,
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }
    }
  }
}
