package com.kvdownload.app.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Audiotrack
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import com.kvdownload.app.ui.theme.LocalAstryxPalette

enum class AstryxTone {
    Accent,
    Success,
    Info,
    Warning,
    Error,
    Neutral
}

@Composable
fun AstryxCard(
    modifier: Modifier = Modifier,
    emphasized: Boolean = false,
    onClick: (() -> Unit)? = null,
    contentPadding: PaddingValues = PaddingValues(14.dp),
    content: @Composable ColumnScope.() -> Unit
) {
    val palette = LocalAstryxPalette.current
    val borderColor = if (emphasized) palette.accent.copy(alpha = 0.45f) else palette.borderSubtle
    val shape = RoundedCornerShape(14.dp)

    val clickMod = if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier

    Surface(
        modifier = modifier.then(clickMod),
        shape = shape,
        color = palette.card,
        border = BorderStroke(1.dp, borderColor)
    ) {
        Column(
            modifier = Modifier.padding(contentPadding),
            verticalArrangement = Arrangement.spacedBy(10.dp),
            content = content
        )
    }
}

@Composable
fun AstryxStatusBadge(
    text: String,
    tone: AstryxTone = AstryxTone.Neutral,
    modifier: Modifier = Modifier
) {
    val palette = LocalAstryxPalette.current
    val baseColor = when (tone) {
        AstryxTone.Accent -> palette.accent
        AstryxTone.Success -> palette.success
        AstryxTone.Info -> palette.info
        AstryxTone.Warning -> palette.warning
        AstryxTone.Error -> palette.error
        AstryxTone.Neutral -> palette.textSecondary
    }
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(999.dp),
        color = baseColor.copy(alpha = if (palette.isDark) 0.16f else 0.11f),
        border = BorderStroke(1.dp, baseColor.copy(alpha = 0.28f))
    ) {
        Text(
            text = text,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
            style = MaterialTheme.typography.labelSmall,
            color = if (tone == AstryxTone.Neutral) palette.textSecondary else baseColor,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
fun AstryxGradientButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    enabled: Boolean = true,
    loading: Boolean = false
) {
    val palette = LocalAstryxPalette.current
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier = modifier
            .clip(shape)
            .background(
                brush = if (enabled) palette.accentGradient
                else Brush.horizontalGradient(listOf(palette.muted, palette.muted))
            )
            .alpha(if (enabled) 1f else 0.55f)
            .clickable(enabled = enabled && !loading, onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        contentAlignment = Alignment.Center
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (loading) {
                CircularProgressIndicator(
                    modifier = Modifier.size(16.dp),
                    strokeWidth = 2.dp,
                    color = Color.White
                )
            } else if (icon != null) {
                Icon(
                    imageVector = icon,
                    contentDescription = null,
                    modifier = Modifier.size(18.dp),
                    tint = Color.White
                )
            }
            Text(
                text = text,
                style = MaterialTheme.typography.titleSmall,
                color = if (enabled) Color.White else palette.textSecondary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
fun AstryxStatPill(
    label: String,
    value: String,
    tone: AstryxTone = AstryxTone.Neutral,
    modifier: Modifier = Modifier
) {
    val palette = LocalAstryxPalette.current
    val accent = when (tone) {
        AstryxTone.Accent -> palette.accent
        AstryxTone.Success -> palette.success
        AstryxTone.Info -> palette.info
        AstryxTone.Warning -> palette.warning
        AstryxTone.Error -> palette.error
        AstryxTone.Neutral -> palette.textSecondary
    }
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(10.dp),
        color = palette.muted,
        border = BorderStroke(1.dp, palette.borderSubtle)
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(7.dp)
                    .clip(CircleShape)
                    .background(accent)
            )
            Text(
                text = label,
                style = MaterialTheme.typography.labelMedium,
                color = palette.textSecondary,
                maxLines = 1
            )
            Text(
                text = value,
                style = MaterialTheme.typography.labelMedium.copy(
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Bold
                ),
                color = palette.textPrimary,
                maxLines = 1
            )
        }
    }
}

@Composable
fun AstryxBrandMark(size: Dp = 32.dp) {
    val palette = LocalAstryxPalette.current
    Box(
        modifier = Modifier
            .size(size)
            .clip(RoundedCornerShape(10.dp))
            .background(palette.accentGradient),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            imageVector = Icons.Default.Bolt,
            contentDescription = "KV Download",
            tint = Color.White,
            modifier = Modifier.size(size * 0.62f)
        )
    }
}

@Composable
fun AstryxMediaThumbnail(
    imageUrl: String?,
    mediaType: String = "video",
    badgeText: String? = null,
    durationText: String? = null,
    showPlayOverlay: Boolean = true,
    onClick: (() -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val palette = LocalAstryxPalette.current
    Box(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f)
            .clip(RoundedCornerShape(10.dp))
            .background(palette.muted)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
    ) {
        if (!imageUrl.isNullOrBlank()) {
            AsyncImage(
                model = imageUrl,
                contentDescription = null,
                modifier = Modifier.fillMaxSize(),
                contentScale = ContentScale.Crop
            )
        } else {
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = when (mediaType) {
                        "audio" -> Icons.Default.Audiotrack
                        "photo", "image" -> Icons.Default.Image
                        else -> Icons.Default.PlayArrow
                    },
                    contentDescription = null,
                    modifier = Modifier.size(36.dp),
                    tint = palette.textSecondary
                )
            }
        }

        // Bottom scrim for crisp readable overlays
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(
                    Brush.verticalGradient(
                        0f to Color.Transparent,
                        0.55f to Color.Transparent,
                        1f to Color.Black.copy(alpha = 0.62f)
                    )
                )
        )

        if (!badgeText.isNullOrBlank()) {
            Surface(
                modifier = Modifier
                    .align(Alignment.TopStart)
                    .padding(8.dp),
                shape = RoundedCornerShape(999.dp),
                color = Color.Black.copy(alpha = 0.68f)
            ) {
                Text(
                    text = badgeText,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                    color = Color.White,
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }

        if (!durationText.isNullOrBlank()) {
            Surface(
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .padding(8.dp),
                shape = RoundedCornerShape(6.dp),
                color = Color.Black.copy(alpha = 0.78f)
            ) {
                Text(
                    text = durationText,
                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                    color = Color.White,
                    fontSize = 11.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        if (showPlayOverlay && onClick != null) {
            Box(
                modifier = Modifier
                    .align(Alignment.Center)
                    .size(44.dp)
                    .clip(CircleShape)
                    .background(Color.Black.copy(alpha = 0.58f)),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = when (mediaType) {
                        "photo", "image" -> Icons.Default.Image
                        "audio" -> Icons.Default.Audiotrack
                        else -> Icons.Default.PlayArrow
                    },
                    contentDescription = "Open media",
                    tint = Color.White,
                    modifier = Modifier.size(24.dp)
                )
            }
        }
    }
}
