package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
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
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

private val SheetBlack = Color(0xFF111111)
private val SheetGray = Color(0xFF272727)
private val SheetLine = Color(0xFF414141)
private val SheetWhite = Color(0xFFF5F5F5)
private val SheetMuted = Color(0xFFAAAAAA)

/** A compact, deliberately neutral control surface for advanced session actions. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SessionControlsDialog(
  initialTitle: String,
  initialProject: String,
  onDismiss: () -> Unit,
  onSaveMetadata: (title: String, project: String) -> Unit,
  onAction: (action: String, body: String) -> Unit,
  onGit: (resource: String) -> Unit,
  onGitWrite: (action: String, body: String) -> Unit,
) {
  var commitMessage by remember { mutableStateOf("") }
  var mergeBranch by remember { mutableStateOf("") }
  var title by remember(initialTitle) { mutableStateOf(initialTitle) }
  var project by remember(initialProject) { mutableStateOf(initialProject) }

  ModalBottomSheet(
    onDismissRequest = onDismiss,
    sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    containerColor = SheetBlack,
    contentColor = SheetWhite,
  ) {
    LazyColumn(
      modifier = Modifier.fillMaxWidth().background(SheetBlack),
      contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 28.dp),
      verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
      item {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
          Column {
            Text("Session controls", fontWeight = FontWeight.SemiBold)
            Text("Manage this Pi session", color = SheetMuted, style = androidx.compose.material3.MaterialTheme.typography.bodySmall)
          }
          TextButton(onClick = onDismiss) { Text("Done", color = SheetWhite) }
        }
      }
      item { SectionLabel("DETAILS") }
      item {
        OutlinedTextField(
          value = title, onValueChange = { title = it }, label = { Text("Title", color = SheetMuted) }, singleLine = true,
          modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors(),
        )
      }
      item {
        OutlinedTextField(
          value = project, onValueChange = { project = it }, label = { Text("Project", color = SheetMuted) }, singleLine = true,
          modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors(),
        )
      }
      item {
        Button(onClick = { onSaveMetadata(title.trim(), project.trim()) }, modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = SheetWhite, contentColor = SheetBlack)) {
          Text("Save details")
        }
      }
      item { HorizontalDivider(color = SheetLine) }
      item { SectionLabel("AGENT") }
      item {
        ControlRow(
          first = "Cycle model" to { onAction("cycle-model", "{}") },
          second = "Thinking / effort" to { onAction("cycle-thinking-level", "{}") },
        )
      }
      item {
        ControlRow(
          first = "Auto compact" to { onAction("auto-compaction", "{\"enabled\":true}") },
          second = "Auto retry" to { onAction("auto-retry", "{\"enabled\":true}") },
        )
      }
      item { HorizontalDivider(color = SheetLine) }
      item { SectionLabel("SESSION") }
      item { ControlRow("Fork" to { onAction("fork", "{}") }, "Clone" to { onAction("clone", "{}") }) }
      item { ControlRow("New session" to { onAction("new", "{}") }, "Rename" to { onAction("name", buildJsonObject { put("name", title) }.toString()) }) }
      item { ControlRow("Switch" to { onAction("switch", "{}") }, "Abort bash" to { onAction("abort-bash", "{}") }) }
      item { HorizontalDivider(color = SheetLine) }
      item { SectionLabel("GIT") }
      item { ControlRow("Status" to { onGit("status") }, "Diff" to { onGit("diff") }) }
      item { ControlRow("Log" to { onGit("log") }, "HEAD" to { onGit("head") }) }
      item { ControlRow("Branches" to { onGit("branches") }, "Worktrees" to { onGit("worktrees") }) }
      item {
        OutlinedTextField(value = commitMessage, onValueChange = { commitMessage = it }, label = { Text("Commit message", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
      }
      item { NeutralAction("Commit all changes", { if (commitMessage.isNotBlank()) onGitWrite("commit", buildJsonObject { put("message", commitMessage); put("stageAll", true) }.toString()) }, Modifier.fillMaxWidth()) }
      item {
        OutlinedTextField(value = mergeBranch, onValueChange = { mergeBranch = it }, label = { Text("Branch to merge", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
      }
      item { NeutralAction("Merge branch", { if (mergeBranch.isNotBlank()) onGitWrite("merge", buildJsonObject { put("branch", mergeBranch) }.toString()) }, Modifier.fillMaxWidth()) }
    }
  }
}

@Composable
private fun SectionLabel(text: String) = Text(text, color = SheetMuted, style = androidx.compose.material3.MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold)

@Composable
private fun ControlRow(first: Pair<String, () -> Unit>, second: Pair<String, (() -> Unit)>?) {
  Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
    NeutralAction(first.first, first.second, Modifier.weight(1f))
    if (second != null) NeutralAction(second.first, second.second, Modifier.weight(1f)) else androidx.compose.foundation.layout.Spacer(Modifier.width(0.dp).weight(1f))
  }
}

@Composable
private fun NeutralAction(label: String, action: () -> Unit, modifier: Modifier = Modifier) {
  Button(
    onClick = action,
    modifier = modifier,
    colors = ButtonDefaults.buttonColors(containerColor = SheetGray, contentColor = SheetWhite),
    contentPadding = PaddingValues(vertical = 11.dp, horizontal = 8.dp),
  ) { Text(label, maxLines = 1, style = androidx.compose.material3.MaterialTheme.typography.labelLarge) }
}

@Composable
private fun neutralFieldColors() = OutlinedTextFieldDefaults.colors(
  focusedContainerColor = SheetGray,
  unfocusedContainerColor = SheetGray,
  focusedTextColor = SheetWhite,
  unfocusedTextColor = SheetWhite,
  focusedBorderColor = SheetWhite,
  unfocusedBorderColor = SheetLine,
  cursorColor = SheetWhite,
)
