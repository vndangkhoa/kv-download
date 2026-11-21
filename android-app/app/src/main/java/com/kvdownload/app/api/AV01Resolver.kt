package com.kvdownload.app.api

import com.google.gson.JsonParser
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Request
import java.net.URI
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.util.Base64
import java.util.concurrent.TimeUnit

data class AV01ResolvedMedia(
    val videoId: String,
    val title: String,
    val code: String?,
    val thumbnail: String?,
    val masterUrl: String,
    val variants: List<StreamVariant>,
    val referer: String = AV01Resolver.REFERER
)

object AV01Resolver {
    const val REFERER = "https://av01.media/"
    const val ORIGIN = "https://av01.media"
    private const val GEO_URL = "https://files.iw01.xyz/edge/geo.js?json"
    private const val API_BASE = "https://customers.iw01.xyz"
    private const val USER_AGENT =
        "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36"

    private val VIDEO_URL_REGEX =
        Regex("""^https?://(?:www\.)?av01\.(?:media|tv)/(?:[a-z]{2}(?:-[a-z]{2})?/)?video/(\d+)(?:/([^/?#]+))?""", RegexOption.IGNORE_CASE)
    private val MANIFEST_URL_REGEX =
        Regex("""^https?://(?:[a-z0-9-]+\.)?iw01\.xyz/api/v1/videos/(\d+)/manifest/""", RegexOption.IGNORE_CASE)
    private val RESOLUTION_REGEX = Regex("""RESOLUTION=(\d+)x(\d+)""", RegexOption.IGNORE_CASE)
    private val BANDWIDTH_REGEX = Regex("""BANDWIDTH=(\d+)""", RegexOption.IGNORE_CASE)

    fun isAV01Url(url: String): Boolean {
        val trimmed = url.trim()
        return VIDEO_URL_REGEX.containsMatchIn(trimmed) || MANIFEST_URL_REGEX.containsMatchIn(trimmed)
    }

    fun parseVideoIdAndSlug(url: String): Pair<String, String?>? {
        val trimmed = url.trim()
        VIDEO_URL_REGEX.find(trimmed)?.let { m ->
            val id = m.groupValues[1].trim()
            val slug = m.groupValues.getOrNull(2)?.trim()?.ifBlank { null }
            if (id.isNotBlank()) return id to slug
        }
        MANIFEST_URL_REGEX.find(trimmed)?.let { m ->
            val id = m.groupValues[1].trim()
            if (id.isNotBlank()) return id to null
        }
        return null
    }

    suspend fun resolve(url: String): AV01ResolvedMedia = withContext(Dispatchers.IO) {
        val (videoId, slug) = parseVideoIdAndSlug(url)
            ?: throw IllegalArgumentException("Not a recognized av01.media video URL")

        val client = RetrofitClient.okHttpClient

        // 1. Fetch geo token from files.iw01.xyz/edge/geo.js?json
        val geoReq = Request.Builder()
            .url(GEO_URL)
            .header("Origin", ORIGIN)
            .header("Referer", REFERER)
            .header("User-Agent", USER_AGENT)
            .header("Accept", "*/*")
            .build()

        val geoJsonStr = client.newCall(geoReq).execute().use { resp ->
            if (!resp.isSuccessful) throw IllegalStateException("AV01 geo HTTP ${resp.code}")
            resp.body?.string() ?: throw IllegalStateException("Empty geo response")
        }
        val geoObj = JsonParser.parseString(geoJsonStr).asJsonObject
        val tokenV2 = geoObj.get("token_v2")?.takeIf { !it.isJsonNull }?.asString.orEmpty()
        val expires = geoObj.get("expires")?.takeIf { !it.isJsonNull }?.asString?.trim().orEmpty()
        val ip = geoObj.get("ip")?.takeIf { !it.isJsonNull }?.asString.orEmpty()
        val comp = geoObj.get("comp")?.takeIf { !it.isJsonNull }?.asBoolean ?: false
        if (tokenV2.isBlank() || expires.isBlank()) {
            throw IllegalStateException("Incomplete AV01 geo token response")
        }

        // 2. Exchange for IP-bound access_token JWT on customers.iw01.xyz
        val encToken = URLEncoder.encode(tokenV2, StandardCharsets.UTF_8.name())
        val cdnAccessUrl = buildString {
            append("$API_BASE/api/v1/videos/$videoId/cdn-access?token_v2=$encToken&expires=$expires")
            if (ip.isNotBlank()) append("&ip=${URLEncoder.encode(ip, StandardCharsets.UTF_8.name())}")
            if (comp) append("&comp=true")
        }
        val accessReq = Request.Builder()
            .url(cdnAccessUrl)
            .header("Origin", ORIGIN)
            .header("Referer", REFERER)
            .header("User-Agent", USER_AGENT)
            .build()

        val accessJsonStr = client.newCall(accessReq).execute().use { resp ->
            if (!resp.isSuccessful) throw IllegalStateException("AV01 cdn-access HTTP ${resp.code}")
            resp.body?.string() ?: throw IllegalStateException("Empty cdn-access response")
        }
        val accessToken = JsonParser.parseString(accessJsonStr).asJsonObject
            .get("access_token")?.takeIf { !it.isJsonNull }?.asString?.trim().orEmpty()
        if (accessToken.isBlank()) {
            throw IllegalStateException("Missing access_token in AV01 cdn-access response")
        }

        val code = extractCodeFromJwt(accessToken)
        var title = deriveTitle(videoId, slug, code)
        var thumbnail: String? = null

        // 3. Best-effort localized metadata lookup on av01.media (short timeout in case ISP blocks it)
        try {
            val metaClient = client.newBuilder()
                .callTimeout(2500, TimeUnit.MILLISECONDS)
                .connectTimeout(2000, TimeUnit.MILLISECONDS)
                .readTimeout(2000, TimeUnit.MILLISECONDS)
                .build()
            val metaReq = Request.Builder()
                .url("https://av01.media/api/v1/videos/$videoId")
                .header("Origin", ORIGIN)
                .header("Referer", REFERER)
                .header("User-Agent", USER_AGENT)
                .build()
            metaClient.newCall(metaReq).execute().use { metaResp ->
                if (metaResp.isSuccessful) {
                    val body = metaResp.body?.string()
                    if (!body.isNullOrBlank()) {
                        val obj = JsonParser.parseString(body).asJsonObject
                        val t = obj.get("title")?.takeIf { !it.isJsonNull }?.asString?.trim()
                            ?: obj.get("name")?.takeIf { !it.isJsonNull }?.asString?.trim()
                        if (!t.isNullOrBlank()) title = t
                        val cover = listOf("cover", "poster", "thumbnail")
                            .firstNotNullOfOrNull { key ->
                                obj.get(key)?.takeIf { !it.isJsonNull }?.asString?.trim()
                                    ?.takeIf { it.startsWith("http://") || it.startsWith("https://") }
                            }
                        if (cover != null) thumbnail = cover
                    }
                }
            }
        } catch (_: Exception) {
        }

        // 4. Fetch master.m3u8 manifest
        val manifestBaseDir = "$API_BASE/api/v1/videos/$videoId/manifest/"
        val encodedJwt = URLEncoder.encode(accessToken, StandardCharsets.UTF_8.name())
        val masterUrl = "${manifestBaseDir}master.m3u8?access_token=$encodedJwt"
        val masterReq = Request.Builder()
            .url(masterUrl)
            .header("Origin", ORIGIN)
            .header("Referer", REFERER)
            .header("User-Agent", USER_AGENT)
            .build()

        val manifestText = client.newCall(masterReq).execute().use { resp ->
            if (!resp.isSuccessful) throw IllegalStateException("AV01 manifest HTTP ${resp.code}")
            resp.body?.string() ?: throw IllegalStateException("Empty master.m3u8")
        }
        if (!manifestText.contains("#EXTM3U")) {
            throw IllegalStateException("AV01 returned non-M3U8 manifest")
        }

        val variants = parseMasterM3U8(manifestText, manifestBaseDir, accessToken, masterUrl)
        if (variants.isEmpty()) {
            throw IllegalStateException("No playable HLS variants found for AV01 #$videoId")
        }

        AV01ResolvedMedia(
            videoId = videoId,
            title = title,
            code = code,
            thumbnail = thumbnail,
            masterUrl = masterUrl,
            variants = variants,
            referer = REFERER
        )
    }

    fun extractCodeFromJwt(jwt: String): String? {
        return try {
            val parts = jwt.trim().split('.')
            if (parts.size < 2) return null
            val payloadBytes = Base64.getUrlDecoder().decode(parts[1])
            val payloadJson = JsonParser.parseString(String(payloadBytes, StandardCharsets.UTF_8)).asJsonObject
            val sub = payloadJson.get("sub")?.takeIf { !it.isJsonNull }?.asString ?: return null
            val segs = sub.split('/').filter { it.isNotBlank() }
            if (segs.size >= 3 && !segs[2].equals("map", ignoreCase = true)) {
                segs[2]
            } else {
                segs.firstOrNull { it.contains('-') && it.length >= 4 }
            }
        } catch (_: Exception) {
            null
        }
    }

    fun deriveTitle(videoId: String, slug: String?, code: String?): String {
        val cleanCode = code?.trim()?.ifBlank { null }
        val cleanSlug = slug?.trim()?.replace('-', ' ')?.replace('_', ' ')?.ifBlank { null }
        return when {
            cleanCode != null && cleanSlug != null -> {
                if (cleanSlug.contains(cleanCode.replace('-', ' '), ignoreCase = true)) {
                    cleanCode.uppercase()
                } else {
                    "$cleanCode · $cleanSlug"
                }
            }
            cleanCode != null -> cleanCode
            cleanSlug != null -> cleanSlug.uppercase()
            else -> "AV01 #$videoId"
        }
    }

    fun parseMasterM3U8(
        content: String,
        manifestBaseDir: String,
        accessToken: String,
        masterUrl: String = ""
    ): List<StreamVariant> {
        val lines = content.replace("\r\n", "\n").lines()
        val variants = mutableListOf<StreamVariant>()

        var pendingW = 0
        var pendingH = 0
        var pendingBw = 0L
        var awaitingUri = false

        val encodedToken = if (accessToken.isNotBlank()) {
            URLEncoder.encode(accessToken, StandardCharsets.UTF_8.name())
        } else ""

        val baseUri = URI.create(manifestBaseDir)

        for (rawLine in lines) {
            val line = rawLine.trim()
            if (line.isBlank()) continue
            if (line.startsWith("#EXT-X-STREAM-INF:")) {
                awaitingUri = true
                pendingW = 0
                pendingH = 0
                pendingBw = 0L
                RESOLUTION_REGEX.find(line)?.let { rm ->
                    pendingW = rm.groupValues[1].toIntOrNull() ?: 0
                    pendingH = rm.groupValues[2].toIntOrNull() ?: 0
                }
                BANDWIDTH_REGEX.find(line)?.let { bm ->
                    pendingBw = bm.groupValues[1].toLongOrNull() ?: 0L
                }
                continue
            }
            if (line.startsWith("#")) continue
            if (awaitingUri) {
                var resolvedUrl = if (line.startsWith("http://") || line.startsWith("https://")) {
                    line
                } else {
                    baseUri.resolve(line).toString()
                }
                if (encodedToken.isNotBlank() && !resolvedUrl.contains("access_token=")) {
                    resolvedUrl += if (resolvedUrl.contains('?')) "&access_token=$encodedToken" else "?access_token=$encodedToken"
                }
                val label = when {
                    pendingH > 0 -> "${pendingH}p"
                    pendingBw > 0 -> "${pendingBw / 1000}k"
                    else -> "Auto"
                }
                variants.add(
                    StreamVariant(
                        label = label,
                        url = resolvedUrl,
                        width = pendingW,
                        height = pendingH,
                        bandwidth = pendingBw
                    )
                )
                awaitingUri = false
            }
        }

        if (variants.isEmpty() && content.contains("#EXTINF:") && masterUrl.isNotBlank()) {
            variants.add(StreamVariant(label = "Auto", url = masterUrl))
        }

        return variants.sortedWith(
            compareByDescending<StreamVariant> { it.height }.thenByDescending { it.bandwidth }
        )
    }

    fun selectVariant(variants: List<StreamVariant>, formatPref: String): StreamVariant {
        val sorted = variants.sortedWith(
            compareByDescending<StreamVariant> { it.height }.thenByDescending { it.bandwidth }
        )
        if (sorted.isEmpty()) throw IllegalArgumentException("No stream variants")
        val targetMax = when (formatPref.lowercase().trim()) {
            "1080", "1080p", "fhd", "high" -> 1080
            "720", "720p", "hd", "normal" -> 720
            "480", "480p", "sd", "low" -> 480
            "360", "360p", "saver" -> 360
            else -> Int.MAX_VALUE
        }
        return if (targetMax == Int.MAX_VALUE) {
            sorted.first()
        } else {
            sorted.firstOrNull { it.height in 1..targetMax } ?: sorted.last()
        }
    }
}
