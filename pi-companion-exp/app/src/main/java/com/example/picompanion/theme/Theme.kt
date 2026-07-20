package com.example.picompanion.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable

private val DarkColorScheme =
  darkColorScheme(
    primary = White,
    onPrimary = Black,
    primaryContainer = Gray800,
    onPrimaryContainer = Gray50,
    secondary = Gray200,
    onSecondary = Black,
    background = Gray950,
    onBackground = Gray50,
    surface = Gray900,
    onSurface = Gray50,
    surfaceVariant = Gray850,
    onSurfaceVariant = Gray200,
    outline = Gray700,
    outlineVariant = Gray800,
    error = Gray200,
    onError = Black,
  )

private val LightColorScheme =
  lightColorScheme(
    primary = Black,
    onPrimary = White,
    primaryContainer = Gray200,
    onPrimaryContainer = Gray950,
    secondary = Gray800,
    onSecondary = White,
    background = Gray50,
    onBackground = Gray950,
    surface = White,
    onSurface = Gray950,
    surfaceVariant = Gray100,
    onSurfaceVariant = Gray700,
    outline = Gray200,
    outlineVariant = Gray200,
    error = Gray900,
    onError = White,
  )

@Composable
fun PiCompanionTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
  MaterialTheme(
    colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme,
    typography = Typography,
    content = content,
  )
}
