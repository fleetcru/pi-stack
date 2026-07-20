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

  fun putToken(serverId: String, token: String) {
    prefs.edit().putString(serverId, token).apply()
  }

  fun removeToken(serverId: String) {
    prefs.edit().remove(serverId).apply()
  }

  /** Strips tokens from server entries and stores them securely. */
  fun migrateTokens(servers: List<ServerEntry>): List<ServerEntry> {
    val editor = prefs.edit()
    for (server in servers) {
      if (server.authToken.isNotBlank()) {
        editor.putString(server.id, server.authToken)
      }
    }
    editor.apply()
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
