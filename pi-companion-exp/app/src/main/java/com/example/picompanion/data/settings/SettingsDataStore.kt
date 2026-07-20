package com.example.picompanion.data.settings

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onStart
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "pi_companion_settings")

class SettingsDataStore(private val context: Context) {

  private val json = Json { ignoreUnknownKeys = true }
  private val secureTokens = SecureTokenStore(context)

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
  }

  private val defaultSettings = AppSettings()

  val settingsFlow: Flow<AppSettings> = context.dataStore.data
    .onStart { migrateTokensIfNeeded() }
    .map { prefs ->
    var servers = try {
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
    )
  }

  suspend fun updateServers(servers: List<ServerEntry>) {
    // Strip tokens before writing to plaintext DataStore.
    val stripped = servers.map { it.copy(authToken = "") }
    context.dataStore.edit {
      it[Keys.SERVERS_JSON] = json.encodeToString(stripped)
    }
    // Store tokens securely, keyed by server ID.
    for (server in servers) {
      if (server.authToken.isNotBlank()) {
        secureTokens.putToken(server.id, server.authToken)
      } else {
        secureTokens.removeToken(server.id)
      }
    }
  }

  /**
   * One-time migration: move plaintext tokens from DataStore to
   * EncryptedSharedPreferences. Safe to call repeatedly — it's idempotent.
   * Must NOT be called from inside a DataStore read transform (deadlock).
   */
  suspend fun migrateTokensIfNeeded() {
    val prefs = context.dataStore.data.first()
    if (prefs[Keys.TOKENS_MIGRATED] == true) return
    var servers = try {
      prefs[Keys.SERVERS_JSON]?.let { json.decodeFromString<List<ServerEntry>>(it) }
    } catch (_: Exception) {
      null
    } ?: return
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
}
