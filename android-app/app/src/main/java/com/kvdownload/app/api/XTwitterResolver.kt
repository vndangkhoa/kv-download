package com.kvdownload.app.api

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Request
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.util.Locale

data class XStatusRef(
    val user: String,
    val tweetId: String,
    val videoIndex: Int = 0
)

data class XVideoItem(
    val id: String,
    val title: String,
    val url: String,
    val thumbnail: String? = null,
    val durationSec: Double = 0.0,
    val width: Int = 0,
    val height: Int = 0,
    val isGif: Boolean = false,
    val variants: List<StreamVariant> = emptyList()
)

data class XResolvedTweet(
    val tweetId: String,
    val canonicalUrl: String,
    val title: String,
    val authorName: String? = null,
    val screenName: String? = null,
    val channel: String,
    val thumbnail: String? = null,
    val videos: List<XVideoItem>,
    val referer: String = XTwitterResolver.REFERER
)

object XTwitterResolver {
    const val REFERER = "https://x.com/"
    const val USER_AGENT = "KV-Download/2.0"

    private const val FX_API_BASE = "https://api.fxtwitter.com"
    private const val VX_API_BASE = "https://api.vxtwitter.com"

    private val STATUS_URL_REGEX = Regex(
        """^https?://(?:(?:www|mobile|m)\.)?(?:x\.com|twitter\.com|fxtwitter\.com|vxtwitter\.com|fixupx\.com|fixvx\.com)/(?:i/web|([^/?#]+))/status(?:es)?/(\d+)(?:/video/(\d+))?(?:[/?#]|$)""",
        RegexOption.IGNORE_CASE
    )
    private val VARIANT_DIM_REGEX = Regex("""/(\d{2,4})x(\d{2,4})/""")
    private val TCO_REGEX = Regex("""https?://t\.co/\S+""")
    private val PIC_REGEX = Regex("""pic\.(?:twitter|x)\.com/\S+""", RegexOption.IGNORE_CASE)
    private val MULTI_SPACE_REGEX = Regex("""\s+""")

    fun isXStatusUrl(url: String): Boolean = parseStatusUrl(url) != null

    fun parseStatusUrl(url: String): XStatusRef? {
        val trimmed = url.trim()
        val match = STATUS_URL_REGEX.find(trimmed) ?: return null
        val rawUser = match.groupValues.getOrNull(1)?.trim().orEmpty()
        val tweetId = match.groupValues.getOrNull(2)?.trim().orEmpty()
        if (tweetId.isBlank()) return null
        val user = if (rawUser.isBlank() || rawUser.equals("i", true) || rawUser.equals("web", true)) {
            "i"
        } else {
            rawUser
        }
        val videoIndex = match.groupValues.getOrNull(3)?.trim()?.toIntOrNull()?.takeIf { it > 0 } ?: 0
        return XStatusRef(user = user, tweetId = tweetId, videoIndex = videoIndex)
    }

    suspend fun resolve(url: String): XResolvedTweet = withContext(Dispatchers.IO) {
        val ref = parseStatusUrl(url)
            ?: throw IllegalArgumentException("Not a recognized X / Twitter status URL")

        val client = RetrofitClient.okHttpClient
        val encUser = URLEncoder.encode(ref.user, StandardCharsets.UTF_8.name())
        val encId = URLEncoder.encode(ref.tweetId, StandardCharsets.UTF_8.name())

        var fxError: Throwable? = null
        try {
            val fxReq = Request.Builder()
                .url("$FX_API_BASE/$encUser/status/$encId")
                .header("User-Agent", USER_AGENT)
                .header("Accept", "application/json")
                .build()
            client.newCall(fxReq).execute().use { resp ->
                if (!resp.isSuccessful) {
                    throw IllegalStateException("FxTwitter HTTP ${resp.code}")
                }
                val body = resp.body?.string().orEmpty()
                val parsed = parseFxTwitterJson(body, ref)
                if (parsed.videos.isNotEmpty()) return@withContext parsed
            }
        } catch (t: Throwable) {
            fxError = t
        }

        try {
            val vxReq = Request.Builder()
                .url("$VX_API_BASE/$encUser/status/$encId")
                .header("User-Agent", USER_AGENT)
                .header("Accept", "application/json")
                .build()
            client.newCall(vxReq).execute().use { resp ->
                if (!resp.isSuccessful) {
                    throw IllegalStateException("VxTwitter HTTP ${resp.code}")
                }
                val body = resp.body?.string().orEmpty()
                val parsed = parseVxTwitterJson(body, ref)
                if (parsed.videos.isNotEmpty()) return@withContext parsed
            }
        } catch (t: Throwable) {
            throw IllegalStateException(
                "Unable to resolve X video (${fxError?.message ?: "fx failed"}; ${t.message ?: "vx failed"})",
                t
            )
        }

        throw IllegalStateException(fxError?.message ?: "No video found in X / Twitter post")
    }

    internal fun parseFxTwitterJson(jsonStr: String, ref: XStatusRef): XResolvedTweet {
        val root = JsonParser.parseString(jsonStr).asJsonObject
        val tweet = root.getAsJsonObject("tweet")
            ?: throw IllegalStateException("FxTwitter returned empty tweet")

        val rawVideos = collectFxMediaVideos(tweet.getAsJsonObject("media")).ifEmpty {
            tweet.getAsJsonObject("quote")?.let { collectFxMediaVideos(it.getAsJsonObject("media")) }.orEmpty()
        }
        if (rawVideos.isEmpty()) {
            throw IllegalStateException("No video found in FxTwitter tweet")
        }

        val author = tweet.getAsJsonObject("author")
        val screenName = author?.str("screen_name")?.takeIf { it.isNotBlank() }
        val authorName = author?.str("name")?.takeIf { it.isNotBlank() }
        val channel = buildChannel(screenName, authorName)
        val rawText = tweet.str("text").orEmpty()
        val baseTitle = buildTitle(rawText, screenName, ref.tweetId)
        val canonicalUrl = tweet.str("url")?.takeIf { it.isNotBlank() }
            ?: "https://x.com/${ref.user}/status/${ref.tweetId}"

        val videos = mutableListOf<XVideoItem>()
        var firstThumb: String? = null

        rawVideos.forEachIndexed { idx, mediaObj ->
            val fallbackW = mediaObj.int("width")
            val fallbackH = mediaObj.int("height")
            val variants = buildFxVariants(mediaObj, fallbackW, fallbackH)
            if (variants.isEmpty()) return@forEachIndexed
            val best = selectVariant(variants, "highest")
            val itemTitle = if (rawVideos.size > 1) "$baseTitle (${idx + 1}/${rawVideos.size})" else baseTitle
            val thumb = mediaObj.str("thumbnail_url")?.takeIf { it.isNotBlank() }
            if (firstThumb == null && thumb != null) firstThumb = thumb
            val vidId = mediaObj.str("id")?.takeIf { it.isNotBlank() } ?: "${ref.tweetId}_${idx + 1}"
            videos.add(
                XVideoItem(
                    id = vidId,
                    title = itemTitle,
                    url = best.url,
                    thumbnail = thumb,
                    durationSec = mediaObj.double("duration"),
                    width = fallbackW,
                    height = fallbackH,
                    isGif = mediaObj.str("type").equals("gif", ignoreCase = true),
                    variants = variants
                )
            )
        }

        if (videos.isEmpty()) {
            throw IllegalStateException("No playable X video variants found")
        }

        return XResolvedTweet(
            tweetId = ref.tweetId,
            canonicalUrl = canonicalUrl,
            title = baseTitle,
            authorName = authorName,
            screenName = screenName,
            channel = channel,
            thumbnail = firstThumb,
            videos = videos,
            referer = REFERER
        )
    }

    internal fun parseVxTwitterJson(jsonStr: String, ref: XStatusRef): XResolvedTweet {
        val root = JsonParser.parseString(jsonStr).asJsonObject
        val rawItems = collectVxMediaVideos(root.get("media_extended")).ifEmpty {
            root.getAsJsonObject("qrt")?.let { collectVxMediaVideos(it.get("media_extended")) }.orEmpty()
        }
        if (rawItems.isEmpty()) {
            throw IllegalStateException("No video found in VxTwitter tweet")
        }

        val screenName = root.str("user_screen_name")?.takeIf { it.isNotBlank() }
        val authorName = root.str("user_name")?.takeIf { it.isNotBlank() }
        val channel = buildChannel(screenName, authorName)
        val rawText = root.str("text").orEmpty()
        val baseTitle = buildTitle(rawText, screenName, ref.tweetId)
        val canonicalUrl = if (screenName != null) {
            "https://x.com/$screenName/status/${ref.tweetId}"
        } else {
            "https://x.com/${ref.user}/status/${ref.tweetId}"
        }

        val videos = mutableListOf<XVideoItem>()
        var firstThumb: String? = null

        rawItems.forEachIndexed { idx, item ->
            val u = item.str("url")?.takeIf { it.isNotBlank() } ?: return@forEachIndexed
            val sizeObj = item.getAsJsonObject("size")
            val fallbackW = sizeObj?.int("width") ?: 0
            val fallbackH = sizeObj?.int("height") ?: 0
            val isHls = isHlsUrl(u, null)
            val (w, h) = parseVariantDims(u, fallbackW, fallbackH, isHls)
            val variant = StreamVariant(
                label = formatVariantLabel(w, h, 0L, isHls),
                url = u,
                width = w,
                height = h,
                bandwidth = 0L
            )
            val itemTitle = if (rawItems.size > 1) "$baseTitle (${idx + 1}/${rawItems.size})" else baseTitle
            val thumb = item.str("thumbnail_url")?.takeIf { it.isNotBlank() }
            if (firstThumb == null && thumb != null) firstThumb = thumb
            val vidId = item.str("id_str")?.takeIf { it.isNotBlank() } ?: "${ref.tweetId}_${idx + 1}"
            val durMs = item.long("duration_millis")
            videos.add(
                XVideoItem(
                    id = vidId,
                    title = itemTitle,
                    url = u,
                    thumbnail = thumb,
                    durationSec = durMs / 1000.0,
                    width = fallbackW,
                    height = fallbackH,
                    isGif = item.str("type").equals("gif", ignoreCase = true),
                    variants = listOf(variant)
                )
            )
        }

        if (videos.isEmpty()) {
            throw IllegalStateException("No playable VxTwitter video URLs found")
        }

        return XResolvedTweet(
            tweetId = ref.tweetId,
            canonicalUrl = canonicalUrl,
            title = baseTitle,
            authorName = authorName,
            screenName = screenName,
            channel = channel,
            thumbnail = firstThumb,
            videos = videos,
            referer = REFERER
        )
    }

    private fun collectFxMediaVideos(media: JsonObject?): List<JsonObject> {
        if (media == null) return emptyList()
        val videosArr = media.getAsJsonArray("videos")
        if (videosArr != null && videosArr.size() > 0) {
            return videosArr.mapNotNull { it.takeIf { el -> el.isJsonObject }?.asJsonObject }
        }
        val allArr = media.getAsJsonArray("all") ?: return emptyList()
        return allArr.mapNotNull { el ->
            val obj = el.takeIf { it.isJsonObject }?.asJsonObject ?: return@mapNotNull null
            val t = obj.str("type")?.lowercase(Locale.US)
            if (t == "video" || t == "gif") obj else null
        }
    }

    private fun collectVxMediaVideos(el: JsonElement?): List<JsonObject> {
        if (el == null || !el.isJsonArray) return emptyList()
        return el.asJsonArray.mapNotNull { item ->
            val obj = item.takeIf { it.isJsonObject }?.asJsonObject ?: return@mapNotNull null
            val t = obj.str("type")?.lowercase(Locale.US)
            val u = obj.str("url")
            if ((t == "video" || t == "gif") && !u.isNullOrBlank()) obj else null
        }
    }

    private fun buildFxVariants(mediaObj: JsonObject, fallbackW: Int, fallbackH: Int): List<StreamVariant> {
        val seen = linkedSetOf<String>()
        val out = mutableListOf<StreamVariant>()

        fun add(rawUrl: String?, contentType: String?, bitrate: Long) {
            val u = rawUrl?.trim().orEmpty()
            if (u.isBlank() || !seen.add(u)) return
            val isHls = isHlsUrl(u, contentType)
            val (w, h) = parseVariantDims(u, fallbackW, fallbackH, isHls)
            out.add(
                StreamVariant(
                    label = formatVariantLabel(w, h, bitrate, isHls),
                    url = u,
                    width = w,
                    height = h,
                    bandwidth = bitrate
                )
            )
        }

        mediaObj.getAsJsonArray("variants")?.forEach { el ->
            if (el.isJsonObject) {
                val obj = el.asJsonObject
                add(obj.str("url"), obj.str("content_type"), obj.long("bitrate"))
            }
        }
        mediaObj.getAsJsonArray("formats")?.forEach { el ->
            if (el.isJsonObject) {
                val obj = el.asJsonObject
                val ct = when (obj.str("container")?.lowercase(Locale.US)) {
                    "m3u8" -> "application/x-mpegURL"
                    "mp4" -> "video/mp4"
                    else -> null
                }
                add(obj.str("url"), ct, obj.long("bitrate"))
            }
        }
        mediaObj.str("url")?.let { topUrl ->
            add(topUrl, mediaObj.str("format"), 0L)
        }

        return sortVariants(out)
    }

    private fun isHlsUrl(url: String, contentType: String?): Boolean {
        if (contentType?.contains("mpegurl", ignoreCase = true) == true) return true
        val clean = url.substringBefore('?').substringBefore('#').lowercase(Locale.US)
        return clean.endsWith(".m3u8")
    }

    private fun parseVariantDims(url: String, fallbackW: Int, fallbackH: Int, isHls: Boolean): Pair<Int, Int> {
        VARIANT_DIM_REGEX.find(url)?.let { m ->
            val w = m.groupValues[1].toIntOrNull() ?: 0
            val h = m.groupValues[2].toIntOrNull() ?: 0
            if (w > 0 && h > 0) return w to h
        }
        return if (!isHls && fallbackW > 0 && fallbackH > 0) fallbackW to fallbackH else 0 to 0
    }

    private fun effectiveDim(w: Int, h: Int): Int = when {
        w > 0 && h > 0 -> minOf(w, h)
        h > 0 -> h
        else -> w
    }

    private fun formatVariantLabel(w: Int, h: Int, bitrate: Long, isHls: Boolean): String {
        val dim = effectiveDim(w, h)
        return when {
            dim > 0 -> "${dim}p"
            bitrate > 0L -> "${bitrate / 1000L}k"
            isHls -> "HLS"
            else -> "MP4"
        }
    }

    private fun sortVariants(variants: List<StreamVariant>): List<StreamVariant> {
        return variants.sortedWith(
            compareBy<StreamVariant> { isHlsUrl(it.url, null) }
                .thenByDescending { it.bandwidth }
                .thenByDescending { it.width.toLong() * it.height.toLong() }
        )
    }

    fun selectVideo(tweet: XResolvedTweet, videoIndex: Int = 0): XVideoItem? {
        if (tweet.videos.isEmpty()) return null
        return if (videoIndex in 1..tweet.videos.size) tweet.videos[videoIndex - 1] else tweet.videos.first()
    }

    fun selectVariant(variants: List<StreamVariant>, formatPref: String): StreamVariant {
        val sorted = sortVariants(variants)
        if (sorted.isEmpty()) throw IllegalArgumentException("No X video variants")
        val mp4s = sorted.filterNot { isHlsUrl(it.url, null) }
        val pool = mp4s.ifEmpty { sorted }

        val maxDim = when (formatPref.lowercase(Locale.US).trim()) {
            "1080", "1080p", "fhd", "high", "optimal" -> 1080
            "720", "720p", "hd", "normal" -> 720
            "480", "480p", "sd", "low" -> 480
            "360", "360p", "saver" -> 360
            else -> Int.MAX_VALUE
        }
        if (maxDim == Int.MAX_VALUE) return pool.first()
        return pool.firstOrNull {
            val d = effectiveDim(it.width, it.height)
            d in 1..maxDim
        } ?: pool.last()
    }

    private fun buildChannel(screenName: String?, authorName: String?): String = when {
        !screenName.isNullOrBlank() -> "@${screenName.removePrefix("@")}"
        !authorName.isNullOrBlank() -> authorName
        else -> "X / Twitter"
    }

    private fun buildTitle(text: String, screenName: String?, tweetId: String): String {
        val cleaned = MULTI_SPACE_REGEX.replace(
            PIC_REGEX.replace(TCO_REGEX.replace(text, " "), " "),
            " "
        ).trim()
        if (cleaned.isNotBlank()) {
            return if (cleaned.length > 85) cleaned.take(82).trimEnd() + "..." else cleaned
        }
        return if (!screenName.isNullOrBlank()) {
            "@${screenName.removePrefix("@")} — X Video $tweetId"
        } else {
            "X Video $tweetId"
        }
    }

    private fun JsonObject.str(key: String): String? =
        get(key)?.takeIf { !it.isJsonNull && it.isJsonPrimitive }?.asString

    private fun JsonObject.int(key: String): Int =
        get(key)?.takeIf { !it.isJsonNull && it.isJsonPrimitive }?.runCatching { asInt }?.getOrDefault(0) ?: 0

    private fun JsonObject.long(key: String): Long =
        get(key)?.takeIf { !it.isJsonNull && it.isJsonPrimitive }?.runCatching { asLong }?.getOrDefault(0L) ?: 0L

    private fun JsonObject.double(key: String): Double =
        get(key)?.takeIf { !it.isJsonNull && it.isJsonPrimitive }?.runCatching { asDouble }?.getOrDefault(0.0) ?: 0.0
}
