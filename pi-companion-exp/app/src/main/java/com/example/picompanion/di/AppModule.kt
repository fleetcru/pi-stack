package com.example.picompanion.di

import android.content.Context
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.settings.SettingsDataStore

/**
 * Lightweight service locator. Provides singleton instances of shared
 * resources (HTTP client, DataStore) to avoid creating duplicate connection
 * pools and DataStore file handles across ViewModels and composables.
 *
 * Call [init] once from Application.onCreate or the first composable that
 * needs it. All properties are safe to access from any thread after init.
 */
object AppModule {
  private lateinit var appContext: Context

  /** Must be called once before accessing any other property. */
  fun init(context: Context) {
    appContext = context.applicationContext
  }

  /** Singleton HTTP client — shares one connection pool across the app. */
  val client: PiServerClient by lazy { PiServerClient() }

  /** Singleton DataStore — one file handle, one migration path. */
  val settingsDataStore: SettingsDataStore by lazy { SettingsDataStore(appContext) }
}
