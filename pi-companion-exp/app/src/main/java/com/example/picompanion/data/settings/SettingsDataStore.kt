package com.example.picompanion.data.settings

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.withContext
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "pi_companion_settings")

class SettingsDataStore(private val context: Context) {

  private val json = Json { ignoreUnknownKeys = true }
  private val secureTokens = SecureTokenStore(context)
  // Mutex ensures migration runs exactly once, even if called concurrently.
  private val migrationMutex = Mutex()
  @Volatile private var migrationDone = false

  private object Keys {
    val TOKENS_MIGRATED = booleanPreferencesKey("tokens_migrated")
    val SERVERS_JSON = stringPreferencesKey("servers_json")
    val ACTIVE_SERVER_ID = stringPreferencesKey("active_server_id")
    val RECONNECT_ON_LAUNCH = booleanPreferencesKey("reconnect_on_launch")
    val REMEMBER_LAST_SESSION = booleanPreferencesKey("remember_last_session")
    val REPLAY_EVENTS = booleanPreferencesKey("replay_events")
    val SHOW_FILE_CHANGES = booleanPreferencesKey("show_file_changes")
    val SHOW_TOOL_EVENTS = booleanPreferencesKey("show_tool_events")
    val SHOW_DAEMON_EVENTS = booleanPreferencesKey("show_daemon_events")
    val DEFAULT_PROJECT_ROOT = stringPreferencesKey("default_project_root")
    val LAST_SESSION_SERVER_ID = stringPreferencesKey("last_session_server_id")
    val LAST_SESSION_ID = stringPreferencesKey("last_session_id")
  }

  private val defaultSettings = AppSettings()

  val settingsFlow: Flow<AppSettings> = context.dataStore.data
    .onStart { ensureMigrationDone() }
    .map { prefs ->
    val servers = try {
      prefs[Keys.SERVERS_JSON]?.let { json.decodeFromString<List<ServerEntry>>(it) }
    } catch (_: Exception) {
      null
    } ?: defaultSettings.servers
    // Hydrate tokens from the secure store — they're stripped in DataStore.
    val hydrated = secureTokens.hydrateTokens(servers)

    AppSettings(
      servers = hydrated,
      activeServerId = prefs[Keys.ACTIVE_SERVER_ID] ?: defaultSettings.activeServerId,
      reconnectOnLaunch = prefs[Keys.RECONNECT_ON_LAUNCH] ?: true,
      rememberLastSession = prefs[Keys.REMEMBER_LAST_SESSION] ?: true,
      replayEventsSinceLastSeen = prefs[Keys.REPLAY_EVENTS] ?: true,
      showFileChangeEvents = prefs[Keys.SHOW_FILE_CHANGES] ?: true,
      showToolEvents = prefs[Keys.SHOW_TOOL_EVENTS] ?: true,
      showDaemonEvents = prefs[Keys.SHOW_DAEMON_EVENTS] ?: true,
      defaultProjectRoot = prefs[Keys.DEFAULT_PROJECT_ROOT] ?: "",
      lastSessionServerId = prefs[Keys.LAST_SESSION_SERVER_ID] ?: "",
      lastSessionId = prefs[Keys.LAST_SESSION_ID] ?: "",
    )
  }

  suspend fun updateServers(servers: List<ServerEntry>) {
    // Commit credentials first. The subsequent DataStore emission can then be
    // hydrated immediately, rather than briefly exposing a server with a
    // missing token to repositories collecting settingsFlow.
    withContext(Dispatchers.IO) { secureTokens.storeTokens(servers) }
    // Strip tokens before writing to plaintext DataStore.
    val stripped = servers.map { it.copy(authToken = "") }
    context.dataStore.edit {
      it[Keys.SERVERS_JSON] = json.encodeToString(stripped)
    }
  }

  /**
   * One-time migration: move plaintext tokens from DataStore to
   * EncryptedSharedPreferences. Uses a mutex to ensure it runs exactly once
   * and never re-enters DataStore while a read transform is active.
   */
  private suspend fun ensureMigrationDone() {
    if (migrationDone) return
    migrationMutex.withLock {
      if (migrationDone) return // double-check after acquiring lock
      migrateTokensIfNeeded()
      migrationDone = true
    }
  }

  /**
   * Actual migration logic. Reads DataStore, moves tokens to secure storage,
   * writes back stripped entries. Idempotent — safe to call repeatedly.
   */
  private suspend fun migrateTokensIfNeeded() {
    // Read the raw preferences directly (not through settingsFlow) to avoid
    // re-entering the flow's transform chain.
    val prefs = context.dataStore.data.first()
    if (prefs[Keys.TOKENS_MIGRATED] == true) return
    var servers = try {
      prefs[Keys.SERVERS_JSON]?.let { json.decodeFromString<List<ServerEntry>>(it) }
    } catch (_: Exception) {
      null
    } ?: return
    // migrateTokens stores tokens in EncryptedSharedPreferences and returns
    // copies with authToken stripped.
    servers = secureTokens.migrateTokens(servers)
    context.dataStore.edit {
      it[Keys.SERVERS_JSON] = json.encodeToString(servers)
      it[Keys.TOKENS_MIGRATED] = true
    }
  }

  suspend fun setActiveServer(id: String) {
    context.dataStore.edit { it[Keys.ACTIVE_SERVER_ID] = id }
  }

  suspend fun updateReconnectOnLaunch(value: Boolean) {
    context.dataStore.edit { it[Keys.RECONNECT_ON_LAUNCH] = value }
  }

  suspend fun updateRememberLastSession(value: Boolean) {
    context.dataStore.edit { it[Keys.REMEMBER_LAST_SESSION] = value }
  }

  suspend fun updateReplayEvents(value: Boolean) {
    context.dataStore.edit { it[Keys.REPLAY_EVENTS] = value }
  }

  suspend fun updateShowFileChanges(value: Boolean) {
    context.dataStore.edit { it[Keys.SHOW_FILE_CHANGES] = value }
  }

  suspend fun updateShowToolEvents(value: Boolean) {
    context.dataStore.edit { it[Keys.SHOW_TOOL_EVENTS] = value }
  }

  suspend fun updateShowDaemonEvents(value: Boolean) {
    context.dataStore.edit { it[Keys.SHOW_DAEMON_EVENTS] = value }
  }

  suspend fun updateDefaultProjectRoot(value: String) {
    context.dataStore.edit { it[Keys.DEFAULT_PROJECT_ROOT] = value }
  }

  suspend fun setLastSession(serverId: String, sessionId: String) {
    context.dataStore.edit {
      it[Keys.LAST_SESSION_SERVER_ID] = serverId
      it[Keys.LAST_SESSION_ID] = sessionId
    }
  }

  suspend fun clearLastSession() {
    context.dataStore.edit {
      it.remove(Keys.LAST_SESSION_SERVER_ID)
      it.remove(Keys.LAST_SESSION_ID)
    }
  }
}
