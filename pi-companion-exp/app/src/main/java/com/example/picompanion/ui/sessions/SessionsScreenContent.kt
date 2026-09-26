package com.example.picompanion.ui.sessions

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.example.picompanion.data.model.ServerSession

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SessionsScreenContent(
  state: SessionsUiState.Content,
  viewModel: SessionsViewModel,
  searchQuery: String,
  onSessionClick: (String) -> Unit,
  onLongClickSession: (ServerSession) -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  activeListState: LazyListState,
  machineListState: LazyListState,
  globalListState: LazyListState,
) {
        val selectedTab by viewModel.selectedTab.collectAsStateWithLifecycle()

        PullToRefreshBox(
          isRefreshing = state.refreshing,
          onRefresh = { viewModel.refresh(force = true) },
          modifier = Modifier.fillMaxSize(),
        ) {
          Column(Modifier.fillMaxSize()) {
        // Tab row
        Row(
          Modifier
            .fillMaxWidth()
            .padding(bottom = 4.dp),
          horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
          FilterChip(
            selected = selectedTab == SessionTab.Active,
            onClick = { viewModel.selectTab(SessionTab.Active) },
            label = { Text("Active (${state.activeSessions.size})") },
          )
          FilterChip(
            selected = selectedTab == SessionTab.Machine,
            onClick = { viewModel.selectTab(SessionTab.Machine) },
            label = { Text("Local (${state.machineSessions.size})") },
          )
          if (state.globalSessions.isNotEmpty()) {
            FilterChip(
              selected = selectedTab == SessionTab.Global,
              onClick = { viewModel.selectTab(SessionTab.Global) },
              label = { Text("Global (${state.globalSessions.size})") },
            )
          }
        }

        when (selectedTab) {
          SessionTab.Active -> {
            val filteredSessions = remember(searchQuery, state.activeSessions) {
              if (searchQuery.isBlank()) state.activeSessions
              else state.activeSessions.filter {
                (it.title?.contains(searchQuery, ignoreCase = true) == true) ||
                  (it.project?.contains(searchQuery, ignoreCase = true) == true) ||
                  (it.status?.contains(searchQuery, ignoreCase = true) == true) ||
                  it.id.contains(searchQuery, ignoreCase = true)
              }
            }

            val sessionGroups = remember(filteredSessions) { groupSessions(filteredSessions) }

            if (filteredSessions.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No active sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Tap + to create a session in an allowed project folder",
              )
            } else {
              LazyColumn(
                state = activeListState,
                contentPadding = PaddingValues(bottom = 16.dp),
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                sessionGroups.forEach { group ->
                  item(key = "section-${group.title}", contentType = "session_section") {
                    SessionGroupHeader(group.title)
                  }
                  items(group.sessions, key = { "session-${it.id}" }, contentType = { "session_item" }) { session ->
                    SessionListItem(
                      session = session,
                      isSelected = false,
                      onClick = { onSessionClick(session.id) },
                      onLongClick = { onLongClickSession(session) },
                      sharedTransitionScope = sharedTransitionScope,
                      animatedVisibilityScope = animatedVisibilityScope,
                    )
                  }
                }
              }
            }
          }

          SessionTab.Machine -> {
            val filteredMachine = remember(searchQuery, state.machineSessions) {
              if (searchQuery.isBlank()) state.machineSessions
              else state.machineSessions.filter {
                it.id.contains(searchQuery, ignoreCase = true) ||
                  it.cwd.contains(searchQuery, ignoreCase = true)
              }
            }

            if (filteredMachine.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No local sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Sessions from ~/.pi/agent/sessions/ appear here",
              )
            } else {
              LazyColumn(
                state = machineListState,
                contentPadding = PaddingValues(bottom = 16.dp),
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                items(filteredMachine, key = { it.id }, contentType = { "machine_item" }) { session ->
                  MachineSessionListItem(
                    session = session,
                    onClick = { viewModel.openMachineSession(session.id, onSessionClick) },
                  )
                }
              }
            }
          }

          SessionTab.Global -> {
            val filteredGlobal = remember(searchQuery, state.globalSessions) {
              if (searchQuery.isBlank()) state.globalSessions
              else state.globalSessions.filter {
                it.session.title?.contains(searchQuery, ignoreCase = true) == true ||
                  it.session.project?.contains(searchQuery, ignoreCase = true) == true ||
                  it.workerId.contains(searchQuery, ignoreCase = true) ||
                  it.originId.contains(searchQuery, ignoreCase = true)
              }
            }

            if (filteredGlobal.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No global sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Sessions from remote workers appear here",
              )
            } else {
              LazyColumn(
                state = globalListState,
                contentPadding = PaddingValues(bottom = 16.dp),
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                items(filteredGlobal, key = { it.id }, contentType = { "global_item" }) { session ->
                  GlobalSessionListItem(
                    session = session,
                    onClick = { viewModel.attachGlobalSession(session.id, onSessionClick) },
                  )
                }
              }
            }
          }
        }
          }
        }

}
