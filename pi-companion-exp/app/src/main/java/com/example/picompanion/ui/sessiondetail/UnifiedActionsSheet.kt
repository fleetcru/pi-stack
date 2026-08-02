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
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

// Unified dark theme colors
private val SheetBg = Color(0xFF111111)
private val SheetCard = Color(0xFF222222)
private val SheetLine = Color(0xFF3A3A3A)
private val SheetText = Color(0xFFF4F4F4)
private val SheetMuted = Color(0xFF999999)

/**
 * Unified actions sheet that combines model/effort selection with session controls
 * and git actions into a single scrollable bottom sheet with tabbed sections.
 * Replaces the previous 3 separate sheets (ModelControlsSheet, SessionControlsDialog).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UnifiedActionsSheet(
  initialTab: Int = 0,
  initialTitle: String,
  initialProject: String,
  controls: ModelControls,
  onDismiss: () -> Unit,
  onSelectModel: (String, String) -> Unit,
  onSelectThinking: (String) -> Unit,
  onSaveMetadata: (title: String, project: String) -> Unit,
  onAction: (action: String, body: String) -> Unit,
  onGit: (resource: String) -> Unit,
  onGitWrite: (action: String, body: String) -> Unit,
) {
  var selectedTab by remember { mutableStateOf(initialTab) }
  var title by remember(initialTitle) { mutableStateOf(initialTitle) }
  var project by remember(initialProject) { mutableStateOf(initialProject) }
  var commitMessage by remember { mutableStateOf("") }
  var mergeBranch by remember { mutableStateOf("") }
  var worktreeBranch by remember { mutableStateOf("") }
  var worktreePath by remember { mutableStateOf("") }
  var existingWorktreeBranch by remember { mutableStateOf(false) }

  // Model state — derived directly from controls so it updates when models load
  val providers = controls.models.map { it.provider }.distinct()
  var provider by remember(controls.selectedProvider, providers) { mutableStateOf(controls.selectedProvider ?: providers.firstOrNull()) }
  // Reset provider when controls change (e.g., after loadModelControls completes)
  LaunchedEffect(controls.models) {
    if (provider == null && providers.isNotEmpty()) {
      provider = controls.selectedProvider ?: providers.first()
    }
  }
  var providerMenuOpen by remember { mutableStateOf(false) }
  val visibleModels = controls.models.filter { it.provider == provider }

  ModalBottomSheet(
    onDismissRequest = onDismiss,
    sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    containerColor = SheetBg,
    contentColor = SheetText,
  ) {
    Column(
      modifier = Modifier
        .fillMaxWidth()
        .heightIn(max = 680.dp)
        .verticalScroll(rememberScrollState())
        .background(SheetBg)
        .padding(start = 20.dp, end = 20.dp, bottom = 28.dp),
      verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
      // Header
      Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Column {
          Text("Session actions", fontWeight = FontWeight.SemiBold)
          Text("Model, controls & git", color = SheetMuted, style = MaterialTheme.typography.bodySmall)
        }
        TextButton(onClick = onDismiss) { Text("Done", color = SheetText) }
      }

      // Tab row
      Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
      ) {
        TabChip("Model", selectedTab == 0) { selectedTab = 0 }
        TabChip("Session", selectedTab == 1) { selectedTab = 1 }
        TabChip("Git", selectedTab == 2) { selectedTab = 2 }
      }

      HorizontalDivider(color = SheetLine)

      when (selectedTab) {
        0 -> ModelTab(
          controls = controls,
          providers = providers,
          provider = provider,
          visibleModels = visibleModels,
          providerMenuOpen = providerMenuOpen,
          onProviderMenuChange = { providerMenuOpen = it },
          onProviderChange = { provider = it },
          onSelectModel = onSelectModel,
          onSelectThinking = onSelectThinking,
        )
        1 -> SessionTab(
          title = title,
          project = project,
          onTitleChange = { title = it },
          onProjectChange = { project = it },
          onSaveMetadata = { onSaveMetadata(title.trim(), project.trim()) },
          onAction = onAction,
        )
        2 -> GitTab(
          commitMessage = commitMessage,
          mergeBranch = mergeBranch,
          worktreeBranch = worktreeBranch,
          worktreePath = worktreePath,
          existingWorktreeBranch = existingWorktreeBranch,
          onCommitMessageChange = { commitMessage = it },
          onMergeBranchChange = { mergeBranch = it },
          onWorktreeBranchChange = { worktreeBranch = it },
          onWorktreePathChange = { worktreePath = it },
          onExistingWorktreeChange = { existingWorktreeBranch = it },
          onGit = onGit,
          onGitWrite = onGitWrite,
        )
      }
    }
  }
}

@Composable
private fun TabChip(label: String, selected: Boolean, onClick: () -> Unit) {
  FilterChip(
    selected = selected,
    onClick = onClick,
    label = { Text(label, fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal) },
  )
}

// ---------------------------------------------------------------------------
// Model tab
// ---------------------------------------------------------------------------

@Composable
private fun ModelTab(
  controls: ModelControls,
  providers: List<String>,
  provider: String?,
  visibleModels: List<ModelChoice>,
  providerMenuOpen: Boolean,
  onProviderMenuChange: (Boolean) -> Unit,
  onProviderChange: (String) -> Unit,
  onSelectModel: (String, String) -> Unit,
  onSelectThinking: (String) -> Unit,
) {
  SectionLabel("PROVIDER")
  Surface(shape = RoundedCornerShape(12.dp), color = SheetCard, modifier = Modifier.fillMaxWidth()) {
    TextButton(onClick = { onProviderMenuChange(true) }, modifier = Modifier.fillMaxWidth(), contentPadding = PaddingValues(horizontal = 14.dp, vertical = 12.dp)) {
      Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(provider ?: "No providers available", color = SheetText)
        Text("Change", color = SheetMuted, style = MaterialTheme.typography.labelMedium)
      }
    }
    DropdownMenu(expanded = providerMenuOpen, onDismissRequest = { onProviderMenuChange(false) }) {
      providers.forEach { value -> DropdownMenuItem(text = { Text(value) }, onClick = { onProviderChange(value); onProviderMenuChange(false) }) }
    }
  }

  SectionLabel("MODELS")
  if (visibleModels.isEmpty()) {
    Text("No models reported by this provider.", style = MaterialTheme.typography.bodySmall, color = SheetMuted)
  } else {
    visibleModels.forEach { model ->
      val selected = controls.selectedProvider == model.provider && controls.selectedModelId == model.id
      Surface(
        onClick = { onSelectModel(model.provider, model.id) },
        shape = RoundedCornerShape(12.dp),
        color = if (selected) Color(0xFFF0F0F0) else SheetCard,
        modifier = Modifier.fillMaxWidth(),
      ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 11.dp)) {
          Text(model.name, fontWeight = FontWeight.SemiBold, color = if (selected) Color(0xFF111111) else SheetText)
          Text(model.id, style = MaterialTheme.typography.labelSmall, color = if (selected) Color(0xFF555555) else SheetMuted)
        }
      }
    }
  }

  HorizontalDivider(color = SheetLine)
  SectionLabel("THINKING / EFFORT")
  Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
    listOf("off", "low", "medium", "high").forEach { level ->
      FilterChip(selected = controls.thinkingLevel == level, onClick = { onSelectThinking(level) }, label = { Text(level) })
    }
  }
}

// ---------------------------------------------------------------------------
// Session tab
// ---------------------------------------------------------------------------

@Composable
private fun SessionTab(
  title: String,
  project: String,
  onTitleChange: (String) -> Unit,
  onProjectChange: (String) -> Unit,
  onSaveMetadata: () -> Unit,
  onAction: (String, String) -> Unit,
) {
  SectionLabel("DETAILS")
  OutlinedTextField(value = title, onValueChange = onTitleChange, label = { Text("Title", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  OutlinedTextField(value = project, onValueChange = onProjectChange, label = { Text("Project", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  Button(onClick = onSaveMetadata, modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = SheetText, contentColor = SheetBg)) { Text("Save details") }

  HorizontalDivider(color = SheetLine)
  SectionLabel("AGENT")
  ControlRow("Cycle model" to { onAction("cycle-model", "{}") }, "Thinking / effort" to { onAction("cycle-thinking-level", "{}") })
  ControlRow("Auto compact" to { onAction("auto-compaction", "{\"enabled\":true}") }, "Auto retry" to { onAction("auto-retry", "{\"enabled\":true}") })

  HorizontalDivider(color = SheetLine)
  SectionLabel("SESSION")
  ControlRow("Fork" to { onAction("fork", "{}") }, "Clone" to { onAction("clone", "{}") })
  ControlRow("New session" to { onAction("new", "{}") }, "Rename" to { onAction("name", buildJsonObject { put("name", title) }.toString()) })
  ControlRow("Switch" to { onAction("switch", "{}") }, "Abort bash" to { onAction("abort-bash", "{}") })
}

// ---------------------------------------------------------------------------
// Git tab
// ---------------------------------------------------------------------------

@Composable
private fun GitTab(
  commitMessage: String,
  mergeBranch: String,
  worktreeBranch: String,
  worktreePath: String,
  existingWorktreeBranch: Boolean,
  onCommitMessageChange: (String) -> Unit,
  onMergeBranchChange: (String) -> Unit,
  onWorktreeBranchChange: (String) -> Unit,
  onWorktreePathChange: (String) -> Unit,
  onExistingWorktreeChange: (Boolean) -> Unit,
  onGit: (String) -> Unit,
  onGitWrite: (String, String) -> Unit,
) {
  SectionLabel("INFO")
  ControlRow("Status" to { onGit("status") }, "Diff" to { onGit("diff") })
  ControlRow("Log" to { onGit("log") }, "HEAD" to { onGit("head") })
  ControlRow("Branches" to { onGit("branches") }, "Worktrees" to { onGit("worktrees") })

  HorizontalDivider(color = SheetLine)
  SectionLabel("WORKTREE")
  OutlinedTextField(value = worktreeBranch, onValueChange = onWorktreeBranchChange, label = { Text("Branch", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  OutlinedTextField(value = worktreePath, onValueChange = onWorktreePathChange, label = { Text("Path", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  ControlRow("New branch" to { onExistingWorktreeChange(false) }, "Existing branch" to { onExistingWorktreeChange(true) })
  NeutralButton("Create worktree", { if (worktreeBranch.isNotBlank() && worktreePath.isNotBlank()) onGitWrite("worktrees", buildJsonObject { put("branch", worktreeBranch); put("path", worktreePath); put("existingBranch", existingWorktreeBranch) }.toString()) }, Modifier.fillMaxWidth())

  HorizontalDivider(color = SheetLine)
  SectionLabel("COMMIT & MERGE")
  OutlinedTextField(value = commitMessage, onValueChange = onCommitMessageChange, label = { Text("Commit message", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  NeutralButton("Commit all changes", { if (commitMessage.isNotBlank()) onGitWrite("commit", buildJsonObject { put("message", commitMessage); put("stageAll", true) }.toString()) }, Modifier.fillMaxWidth())
  OutlinedTextField(value = mergeBranch, onValueChange = onMergeBranchChange, label = { Text("Branch to merge", color = SheetMuted) }, singleLine = true, modifier = Modifier.fillMaxWidth(), colors = neutralFieldColors())
  NeutralButton("Merge branch", { if (mergeBranch.isNotBlank()) onGitWrite("merge", buildJsonObject { put("branch", mergeBranch) }.toString()) }, Modifier.fillMaxWidth())

  HorizontalDivider(color = SheetLine)
  SectionLabel("REMOTE")
  ControlRow("Pull" to { onGitWrite("pull", "{\"remote\":\"origin\"}") }, "Push" to { onGitWrite("push", "{\"remote\":\"origin\"}") })
  NeutralButton("Abort merge", { onGitWrite("merge-abort", "{}") }, Modifier.fillMaxWidth())
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

@Composable
private fun SectionLabel(text: String) = Text(text, color = SheetMuted, style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold)

@Composable
private fun ControlRow(first: Pair<String, () -> Unit>, second: Pair<String, () -> Unit>?) {
  Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
    NeutralButton(first.first, first.second, Modifier.weight(1f))
    if (second != null) NeutralButton(second.first, second.second, Modifier.weight(1f))
  }
}

@Composable
private fun NeutralButton(label: String, action: () -> Unit, modifier: Modifier = Modifier) {
  Button(
    onClick = action,
    modifier = modifier,
    colors = ButtonDefaults.buttonColors(containerColor = SheetCard, contentColor = SheetText),
    contentPadding = PaddingValues(vertical = 11.dp, horizontal = 8.dp),
  ) { Text(label, maxLines = 1, style = MaterialTheme.typography.labelLarge) }
}

@Composable
private fun neutralFieldColors() = OutlinedTextFieldDefaults.colors(
  focusedContainerColor = SheetCard,
  unfocusedContainerColor = SheetCard,
  focusedTextColor = SheetText,
  unfocusedTextColor = SheetText,
  focusedBorderColor = SheetText,
  unfocusedBorderColor = SheetLine,
  cursorColor = SheetText,
)
