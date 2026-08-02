package com.example.picompanion.data.settings

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Stores auth tokens in EncryptedSharedPreferences backed by Android Keystore.
 * Tokens are keyed by server ID. This keeps sensitive credentials out of the
 * plaintext DataStore used for non-secret settings.
 */
class SecureTokenStore(context: Context) {

  private val masterKey = MasterKey.Builder(context)
    .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
    .build()

  private val prefs: SharedPreferences = EncryptedSharedPreferences.create(
    context,
    "pi_secure_tokens",
    masterKey,
    EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
    EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
  )

  fun getToken(serverId: String): String {
    return prefs.getString(serverId, "") ?: ""
  }

  @Synchronized
  fun putToken(serverId: String, token: String) {
    check(prefs.edit().putString(serverId, token).commit()) { "Failed to persist auth token" }
  }

  @Synchronized
  fun removeToken(serverId: String) {
    check(prefs.edit().remove(serverId).commit()) { "Failed to remove auth token" }
  }

  /**
   * Persists all provided tokens in one synchronous edit. Settings callers use
   * this before publishing the matching server list through DataStore, so a
   * collector never observes a newly configured server without its token.
   */
  @Synchronized
  fun storeTokens(servers: List<ServerEntry>) {
    val editor = prefs.edit()
    for (server in servers) {
      if (server.authToken.isNotBlank()) {
        editor.putString(server.id, server.authToken)
      } else {
        editor.remove(server.id)
      }
    }
    check(editor.commit()) { "Failed to persist auth tokens" }
  }

  /** Strips tokens from server entries and stores them securely. */
  @Synchronized
  fun migrateTokens(servers: List<ServerEntry>): List<ServerEntry> {
    val editor = prefs.edit()
    for (server in servers) {
      if (server.authToken.isNotBlank()) {
        editor.putString(server.id, server.authToken)
      }
    }
    check(editor.commit()) { "Failed to migrate auth tokens" }
    return servers.map { it.copy(authToken = "") }
  }

  /** Merges secure tokens back into server entries. */
  fun hydrateTokens(servers: List<ServerEntry>): List<ServerEntry> {
    return servers.map { server ->
      val token = prefs.getString(server.id, "") ?: ""
      if (token.isNotBlank()) server.copy(authToken = token) else server
    }
  }
}
