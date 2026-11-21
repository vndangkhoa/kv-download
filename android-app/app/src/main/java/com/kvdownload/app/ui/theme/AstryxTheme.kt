package com.kvdownload.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

object AstryxTokens {
    // Dark tokens matching frontend/src/theme/kvDownloadTheme.ts
    val DarkBody = Color(0xFF0B0C12)
    val DarkSurface = Color(0xFF131520)
    val DarkCard = Color(0xFF161926)
    val DarkMuted = Color(0xFF1A1D2B)
    val DarkPopover = Color(0xFF1C2030)
    val DarkAccent = Color(0xFF8B5CF6)
    val DarkTextAccent = Color(0xFFC4B5FD)
    val DarkTextPrimary = Color(0xFFF1F5F9)
    val DarkTextSecondary = Color(0xFF94A3B8)
    val DarkBorder = Color(0x18FFFFFF)
    val DarkBorderStrong = Color(0x33FFFFFF)

    // Light tokens matching frontend/src/theme/kvDownloadTheme.ts
    val LightBody = Color(0xFFF4F6FB)
    val LightSurface = Color(0xFFFFFFFF)
    val LightCard = Color(0xFFFFFFFF)
    val LightMuted = Color(0xFFF1F4F9)
    val LightAccent = Color(0xFF6D28D9)
    val LightTextAccent = Color(0xFF5B21B6)
    val LightTextPrimary = Color(0xFF0F172A)
    val LightTextSecondary = Color(0xFF475569)
    val LightBorder = Color(0x1A0F172A)
    val LightBorderStrong = Color(0x380F172A)

    // Semantic statuses
    val SuccessLight = Color(0xFF059669)
    val SuccessDark = Color(0xFF10B981)
    val InfoLight = Color(0xFF0284C7)
    val InfoDark = Color(0xFF0EA5E9)
    val WarningLight = Color(0xFFD97706)
    val WarningDark = Color(0xFFF59E0B)
    val ErrorLight = Color(0xFFDC2626)
    val ErrorDark = Color(0xFFEF4444)
}

private val DarkColorScheme = darkColorScheme(
    primary = AstryxTokens.DarkAccent,
    onPrimary = Color.White,
    primaryContainer = Color(0xFF2D1F52),
    onPrimaryContainer = AstryxTokens.DarkTextAccent,
    secondary = AstryxTokens.InfoDark,
    onSecondary = Color.White,
    secondaryContainer = AstryxTokens.DarkMuted,
    onSecondaryContainer = AstryxTokens.DarkTextPrimary,
    tertiary = AstryxTokens.SuccessDark,
    background = AstryxTokens.DarkBody,
    onBackground = AstryxTokens.DarkTextPrimary,
    surface = AstryxTokens.DarkSurface,
    onSurface = AstryxTokens.DarkTextPrimary,
    surfaceVariant = AstryxTokens.DarkCard,
    onSurfaceVariant = AstryxTokens.DarkTextSecondary,
    surfaceContainerLowest = AstryxTokens.DarkBody,
    surfaceContainerLow = AstryxTokens.DarkSurface,
    surfaceContainer = AstryxTokens.DarkCard,
    surfaceContainerHigh = AstryxTokens.DarkMuted,
    surfaceContainerHighest = AstryxTokens.DarkPopover,
    outline = AstryxTokens.DarkBorderStrong,
    outlineVariant = AstryxTokens.DarkBorder,
    error = AstryxTokens.ErrorDark,
    onError = Color.White,
)

private val LightColorScheme = lightColorScheme(
    primary = AstryxTokens.LightAccent,
    onPrimary = Color.White,
    primaryContainer = Color(0xFFEDE9FE),
    onPrimaryContainer = AstryxTokens.LightTextAccent,
    secondary = AstryxTokens.InfoLight,
    onSecondary = Color.White,
    secondaryContainer = AstryxTokens.LightMuted,
    onSecondaryContainer = AstryxTokens.LightTextPrimary,
    tertiary = AstryxTokens.SuccessLight,
    background = AstryxTokens.LightBody,
    onBackground = AstryxTokens.LightTextPrimary,
    surface = AstryxTokens.LightSurface,
    onSurface = AstryxTokens.LightTextPrimary,
    surfaceVariant = AstryxTokens.LightMuted,
    onSurfaceVariant = AstryxTokens.LightTextSecondary,
    surfaceContainerLowest = AstryxTokens.LightSurface,
    surfaceContainerLow = AstryxTokens.LightBody,
    surfaceContainer = AstryxTokens.LightCard,
    surfaceContainerHigh = AstryxTokens.LightMuted,
    surfaceContainerHighest = Color(0xFFE2E8F0),
    outline = AstryxTokens.LightBorderStrong,
    outlineVariant = AstryxTokens.LightBorder,
    error = AstryxTokens.ErrorLight,
    onError = Color.White,
)

private val AstryxTypography = Typography(
    titleLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Bold,
        fontSize = 20.sp,
        lineHeight = 26.sp
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 16.sp,
        lineHeight = 22.sp
    ),
    titleSmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 14.sp,
        lineHeight = 20.sp
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 20.sp
    ),
    bodySmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Normal,
        fontSize = 12.sp,
        lineHeight = 16.sp
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 16.sp
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.Monospace,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 14.sp
    )
)

@Composable
fun KVDownloadTheme(
    themeMode: String = "system",
    content: @Composable () -> Unit
) {
    val darkTheme = when (themeMode) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }

    MaterialTheme(
        colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme,
        typography = AstryxTypography,
        content = content
    )
}
