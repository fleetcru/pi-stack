package com.example.picompanion.data.api

import kotlinx.serialization.json.Json

val apiJson: Json = Json {
  ignoreUnknownKeys = true
  isLenient = true
  coerceInputValues = true
}
