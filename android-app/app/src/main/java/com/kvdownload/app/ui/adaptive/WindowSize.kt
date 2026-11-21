package com.kvdownload.app.ui.adaptive

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.window.layout.FoldingFeature
import androidx.window.layout.WindowInfoTracker
import kotlin.math.roundToInt

enum class WidthClass {
    Compact,
    Medium,
    Expanded
}

enum class HeightClass {
    Compact,
    Medium,
    Expanded
}

enum class FoldPosture {
    Flat,
    Tabletop,
    Book
}

data class HingeRectDp(
    val left: Int,
    val top: Int,
    val right: Int,
    val bottom: Int
)

data class WindowState(
    val widthDp: Int = 390,
    val heightDp: Int = 844,
    val widthClass: WidthClass = WidthClass.Compact,
    val heightClass: HeightClass = HeightClass.Medium,
    val foldPosture: FoldPosture = FoldPosture.Flat,
    val hingeBoundsDp: HingeRectDp? = null
) {
    val isLandscape: Boolean get() = widthDp > heightDp
    val isTabletop: Boolean get() = foldPosture == FoldPosture.Tabletop && (hingeBoundsDp?.top ?: 0) > 120
    val isBook: Boolean get() = foldPosture == FoldPosture.Book && (hingeBoundsDp?.left ?: 0) > 200
    val useNavigationRail: Boolean
        get() = widthClass == WidthClass.Expanded ||
            foldPosture == FoldPosture.Book ||
            (widthClass == WidthClass.Medium && isLandscape)
    val isTwoPane: Boolean
        get() = widthClass == WidthClass.Expanded ||
            foldPosture == FoldPosture.Book ||
            (widthClass == WidthClass.Medium && isLandscape)
    val queueGridColumns: Int
        get() = when (widthClass) {
            WidthClass.Compact -> if (isLandscape) 2 else 1
            WidthClass.Medium -> 2
            WidthClass.Expanded -> if (widthDp >= 1180) 3 else 2
        }
    val galleryColumns: Int
        get() = when (widthClass) {
            WidthClass.Compact -> if (isLandscape) 2 else 1
            WidthClass.Medium -> if (isLandscape) 3 else 2
            WidthClass.Expanded -> if (widthDp >= 1200) 4 else 3
        }
}

val LocalWindowState = compositionLocalOf { WindowState() }

fun computeWindowState(
    widthDp: Int,
    heightDp: Int,
    isHalfOpened: Boolean = false,
    isHorizontalHinge: Boolean = false,
    hingeBoundsDp: HingeRectDp? = null
): WindowState {
    val wClass = when {
        widthDp < 600 -> WidthClass.Compact
        widthDp < 840 -> WidthClass.Medium
        else -> WidthClass.Expanded
    }
    val hClass = when {
        heightDp < 480 -> HeightClass.Compact
        heightDp < 900 -> HeightClass.Medium
        else -> HeightClass.Expanded
    }
    val posture = when {
        !isHalfOpened || hingeBoundsDp == null -> FoldPosture.Flat
        isHorizontalHinge -> FoldPosture.Tabletop
        else -> FoldPosture.Book
    }
    return WindowState(
        widthDp = widthDp,
        heightDp = heightDp,
        widthClass = wClass,
        heightClass = hClass,
        foldPosture = posture,
        hingeBoundsDp = hingeBoundsDp
    )
}

@Composable
fun ProvideWindowState(
    activity: Activity?,
    content: @Composable () -> Unit
) {
    val config = LocalConfiguration.current
    val density = LocalDensity.current
    var activeFold by remember { mutableStateOf<FoldingFeature?>(null) }

    LaunchedEffect(activity) {
        if (activity != null) {
            WindowInfoTracker.getOrCreate(activity)
                .windowLayoutInfo(activity)
                .collect { layoutInfo ->
                    activeFold = layoutInfo.displayFeatures
                        .filterIsInstance<FoldingFeature>()
                        .firstOrNull()
                }
        }
    }

    val fold = activeFold
    val isHalfOpened = fold?.state == FoldingFeature.State.HALF_OPENED
    val isHorizontalHinge = fold?.orientation == FoldingFeature.Orientation.HORIZONTAL
    val hingeBoundsDp = fold?.let {
        val d = density.density.coerceAtLeast(1f)
        HingeRectDp(
            left = (it.bounds.left / d).roundToInt(),
            top = (it.bounds.top / d).roundToInt(),
            right = (it.bounds.right / d).roundToInt(),
            bottom = (it.bounds.bottom / d).roundToInt()
        )
    }

    val state = remember(
        config.screenWidthDp,
        config.screenHeightDp,
        isHalfOpened,
        isHorizontalHinge,
        hingeBoundsDp
    ) {
        computeWindowState(
            widthDp = config.screenWidthDp,
            heightDp = config.screenHeightDp,
            isHalfOpened = isHalfOpened,
            isHorizontalHinge = isHorizontalHinge,
            hingeBoundsDp = hingeBoundsDp
        )
    }

    CompositionLocalProvider(LocalWindowState provides state) {
        content()
    }
}

tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
