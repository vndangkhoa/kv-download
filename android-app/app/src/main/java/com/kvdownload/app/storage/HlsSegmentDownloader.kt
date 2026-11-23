package com.kvdownload.app.storage

import android.content.Context
import android.media.MediaCodec
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMuxer
import android.webkit.CookieManager
import com.kvdownload.app.api.RetrofitClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import okhttp3.Request
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.net.URI
import java.net.URLDecoder
import java.nio.ByteBuffer
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import javax.crypto.Cipher
import javax.crypto.spec.IvParameterSpec
import javax.crypto.spec.SecretKeySpec

data class HlsSegmentSpec(
    val index: Int,
    val url: String,
    val keyUrl: String? = null,
    val ivBytes: ByteArray? = null,
    val mediaSequenceNumber: Long = index.toLong()
)

data class HlsPlaylistSpec(
    val initMapUrl: String?,
    val initKeyUrl: String?,
    val initIvBytes: ByteArray?,
    val segments: List<HlsSegmentSpec>,
    val isFmp4: Boolean
)

data class HlsDownloadedFile(
    val file: File,
    val extension: String,
    val mimeType: String,
    val totalBytes: Long
)

object HlsSegmentDownloader {

    private const val DEFAULT_UA =
        "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36"

    private val STREAM_INF_RES_REGEX = Regex("""RESOLUTION=(\d+)x(\d+)""", RegexOption.IGNORE_CASE)
    private val STREAM_INF_BW_REGEX = Regex("""BANDWIDTH=(\d+)""", RegexOption.IGNORE_CASE)
    private val TAG_URI_REGEX = Regex("""URI="([^"]+)"""")
    private val KEY_METHOD_REGEX = Regex("""METHOD=([^,\s]+)""", RegexOption.IGNORE_CASE)
    private val KEY_IV_REGEX = Regex("""IV=0x([0-9a-fA-F]+)""", RegexOption.IGNORE_CASE)

    fun looksLikeHlsUrl(url: String): Boolean {
        val raw = url.trim()
        if (raw.isBlank()) return false
        val pathOnly = raw.substringBefore('?').substringBefore('#').lowercase()
        if (pathOnly.endsWith(".m3u8")) return true
        // Handle server-proxied URL like /api/browser/stream?url=https%3A%2F%2F...master.m3u8
        if (raw.contains("url=")) {
            val encodedInner = raw.substringAfter("url=").substringBefore('&')
            val decoded = try {
                URLDecoder.decode(encodedInner, "UTF-8")
            } catch (_: Exception) {
                encodedInner
            }
            val innerPath = decoded.substringBefore('?').substringBefore('#').lowercase()
            if (innerPath.endsWith(".m3u8")) return true
        }
        return raw.lowercase().contains(".m3u8?")
    }

    fun selectVariantFromMaster(masterText: String, baseUrl: String, preferredFormat: String = "highest"): String {
        val lines = masterText.replace("\r\n", "\n").lines()
        val candidates = mutableListOf<Triple<Int, Long, String>>()
        var pendingH = 0
        var pendingBw = 0L
        var awaitingUri = false
        val baseUri = URI.create(baseUrl)

        for (rawLine in lines) {
            val line = rawLine.trim()
            if (line.isBlank()) continue
            if (line.startsWith("#EXT-X-STREAM-INF:")) {
                awaitingUri = true
                pendingH = STREAM_INF_RES_REGEX.find(line)?.groupValues?.getOrNull(2)?.toIntOrNull() ?: 0
                pendingBw = STREAM_INF_BW_REGEX.find(line)?.groupValues?.getOrNull(1)?.toLongOrNull() ?: 0L
                continue
            }
            if (line.startsWith("#")) continue
            if (awaitingUri) {
                val abs = resolveRef(baseUri, line)
                candidates.add(Triple(pendingH, pendingBw, abs))
                awaitingUri = false
            }
        }
        if (candidates.isEmpty()) return baseUrl
        val sorted = candidates.sortedWith(compareByDescending<Triple<Int, Long, String>> { it.first }.thenByDescending { it.second })
        val maxH = when (preferredFormat.lowercase().trim()) {
            "1080", "1080p", "fhd" -> 1080
            "720", "720p", "hd" -> 720
            "480", "480p", "sd" -> 480
            "360", "360p" -> 360
            else -> Int.MAX_VALUE
        }
        val chosen = if (maxH == Int.MAX_VALUE) {
            sorted.first()
        } else {
            sorted.firstOrNull { it.first in 1..maxH } ?: sorted.last()
        }
        return chosen.third
    }

    fun parseMediaPlaylist(playlistText: String, baseUrl: String): HlsPlaylistSpec {
        val lines = playlistText.replace("\r\n", "\n").lines()
        val baseUri = URI.create(baseUrl)
        val segments = mutableListOf<HlsSegmentSpec>()

        var initMapUrl: String? = null
        var initKeyUrl: String? = null
        var initIvBytes: ByteArray? = null
        var currentKeyUrl: String? = null
        var currentIvBytes: ByteArray? = null
        var mediaSeq = 0L

        for (rawLine in lines) {
            val line = rawLine.trim()
            if (line.isBlank()) continue

            when {
                line.startsWith("#EXT-X-MEDIA-SEQUENCE:") -> {
                    mediaSeq = line.substringAfter(':').trim().toLongOrNull() ?: 0L
                }
                line.startsWith("#EXT-X-KEY:") -> {
                    val method = KEY_METHOD_REGEX.find(line)?.groupValues?.getOrNull(1)?.uppercase() ?: "NONE"
                    if (method == "NONE") {
                        currentKeyUrl = null
                        currentIvBytes = null
                    } else if (method == "AES-128") {
                        val uriRef = TAG_URI_REGEX.find(line)?.groupValues?.getOrNull(1)
                        currentKeyUrl = uriRef?.let { resolveRef(baseUri, it) }
                        val ivHex = KEY_IV_REGEX.find(line)?.groupValues?.getOrNull(1)
                        currentIvBytes = ivHex?.let { parseHexIv(it) }
                    } else {
                        throw IllegalStateException("Unsupported HLS encryption method: $method")
                    }
                }
                line.startsWith("#EXT-X-MAP:") -> {
                    if (initMapUrl == null) {
                        val uriRef = TAG_URI_REGEX.find(line)?.groupValues?.getOrNull(1)
                        if (!uriRef.isNullOrBlank()) {
                            initMapUrl = resolveRef(baseUri, uriRef)
                            initKeyUrl = currentKeyUrl
                            initIvBytes = currentIvBytes
                        }
                    }
                }
                line.startsWith("#") -> Unit
                else -> {
                    val idx = segments.size
                    val segUrl = resolveRef(baseUri, line)
                    segments.add(
                        HlsSegmentSpec(
                            index = idx,
                            url = segUrl,
                            keyUrl = currentKeyUrl,
                            ivBytes = currentIvBytes,
                            mediaSequenceNumber = mediaSeq + idx
                        )
                    )
                }
            }
        }

        val hasFmp4Ext = segments.any {
            val path = it.url.substringBefore('?').substringBefore('#').lowercase()
            path.endsWith(".m4s") || path.endsWith(".mp4") || path.endsWith(".cmfv") || path.endsWith(".cmfa")
        }
        return HlsPlaylistSpec(
            initMapUrl = initMapUrl,
            initKeyUrl = initKeyUrl,
            initIvBytes = initIvBytes,
            segments = segments,
            isFmp4 = initMapUrl != null || hasFmp4Ext
        )
    }

    suspend fun downloadHlsToTempFile(
        context: Context,
        playlistUrl: String,
        referer: String?,
        preferredFormat: String,
        qualityLabel: String?,
        onProgress: (Float, Long, String) -> Unit
    ): HlsDownloadedFile {
        val effReferer = referer?.ifBlank { null }
            ?: if (playlistUrl.contains("iw01.xyz", ignoreCase = true)) "https://av01.media/" else playlistUrl

        onProgress(0.01f, 0L, "Fetching HLS manifest...")
        val firstManifest = fetchTextWithRetry(playlistUrl, effReferer)
        if (!firstManifest.contains("#EXTM3U")) {
            throw IllegalStateException("URL did not return valid #EXTM3U playlist")
        }

        var activePlaylistUrl = playlistUrl
        var activePlaylistText = firstManifest
        if (firstManifest.contains("#EXT-X-STREAM-INF:")) {
            activePlaylistUrl = selectVariantFromMaster(firstManifest, playlistUrl, preferredFormat)
            onProgress(0.03f, 0L, "Loading ${qualityLabel ?: preferredFormat} playlist...")
            activePlaylistText = fetchTextWithRetry(activePlaylistUrl, effReferer)
        }

        val spec = parseMediaPlaylist(activePlaylistText, activePlaylistUrl)
        if (spec.segments.isEmpty()) {
            throw IllegalStateException("HLS playlist has 0 media segments")
        }

        val workDir = File(context.cacheDir, "hls_${UUID.randomUUID()}")
        workDir.mkdirs()

        val tagPrefix = qualityLabel?.takeIf { it.isNotBlank() }?.let { "HLS $it" } ?: "HLS"
        val keyUrls = (listOfNotNull(spec.initKeyUrl) + spec.segments.mapNotNull { it.keyUrl }).distinct()
        val keyCache = mutableMapOf<String, ByteArray>()
        for (kUrl in keyUrls) {
            keyCache[kUrl] = fetchBytesWithRetry(kUrl, effReferer)
        }

        val segFiles = arrayOfNulls<File>(spec.segments.size)
        var initFile: File? = null
        val downloadedBytes = AtomicLong(0L)
        val doneCount = AtomicInteger(0)
        val totalParts = spec.segments.size + if (spec.initMapUrl != null) 1 else 0

        try {
            if (spec.initMapUrl != null) {
                val rawInit = fetchBytesWithRetry(spec.initMapUrl, effReferer)
                val plainInit = decryptIfNeeded(rawInit, spec.initKeyUrl?.let { keyCache[it] }, spec.initIvBytes, 0L)
                initFile = File(workDir, "init.mp4").apply { writeBytes(plainInit) }
                downloadedBytes.addAndGet(plainInit.size.toLong())
                val c = doneCount.incrementAndGet()
                onProgress((c.toFloat() / totalParts).coerceIn(0f, 0.92f), downloadedBytes.get(), "$tagPrefix · init segment")
            }

            val sem = Semaphore(6)
            coroutineScope {
                spec.segments.map { seg ->
                    async(Dispatchers.IO) {
                        sem.withPermit {
                            val rawBytes = fetchBytesWithRetry(seg.url, effReferer)
                            val key = seg.keyUrl?.let { keyCache[it] }
                            val plainBytes = decryptIfNeeded(rawBytes, key, seg.ivBytes, seg.mediaSequenceNumber)
                            val segFile = File(workDir, "seg_%06d.bin".format(seg.index))
                            segFile.writeBytes(plainBytes)
                            segFiles[seg.index] = segFile

                            val bytesNow = downloadedBytes.addAndGet(plainBytes.size.toLong())
                            val c = doneCount.incrementAndGet()
                            val pct = (c.toFloat() / totalParts.toFloat()) * 0.92f
                            onProgress(pct.coerceIn(0f, 0.92f), bytesNow, "$tagPrefix · $c/${spec.segments.size} segs")
                        }
                    }
                }.awaitAll()
            }

            onProgress(0.94f, downloadedBytes.get(), "$tagPrefix · assembling MP4...")
            val finalOutFile: File
            val finalExt: String
            val finalMime: String

            if (spec.isFmp4) {
                val mergedFmp4 = File(workDir, "merged_fmp4.mp4")
                FileOutputStream(mergedFmp4).use { out ->
                    initFile?.let { copyInto(it, out) }
                    for (sf in segFiles) {
                        if (sf != null) copyInto(sf, out)
                    }
                    out.flush()
                }
                val remuxedMp4 = File(context.cacheDir, "kv_hls_out_${UUID.randomUUID()}.mp4")
                if (remuxToStandardMp4(mergedFmp4, remuxedMp4)) {
                    finalOutFile = remuxedMp4
                } else {
                    remuxedMp4.delete()
                    val fallbackMp4 = File(context.cacheDir, "kv_hls_out_${UUID.randomUUID()}.mp4")
                    mergedFmp4.copyTo(fallbackMp4, overwrite = true)
                    finalOutFile = fallbackMp4
                }
                finalExt = "mp4"
                finalMime = "video/mp4"
            } else {
                val mergedTs = File(workDir, "merged.ts")
                FileOutputStream(mergedTs).use { out ->
                    initFile?.let { copyInto(it, out) }
                    for (sf in segFiles) {
                        if (sf != null) copyInto(sf, out)
                    }
                    out.flush()
                }
                val remuxedMp4 = File(context.cacheDir, "kv_hls_out_${UUID.randomUUID()}.mp4")
                if (remuxToStandardMp4(mergedTs, remuxedMp4)) {
                    finalOutFile = remuxedMp4
                    finalExt = "mp4"
                    finalMime = "video/mp4"
                } else {
                    remuxedMp4.delete()
                    val preservedTs = File(context.cacheDir, "kv_hls_out_${UUID.randomUUID()}.ts")
                    mergedTs.copyTo(preservedTs, overwrite = true)
                    finalOutFile = preservedTs
                    finalExt = "ts"
                    finalMime = "video/mp2t"
                }
            }

            onProgress(0.97f, finalOutFile.length(), "Saving to private app vault...")
            return HlsDownloadedFile(
                file = finalOutFile,
                extension = finalExt,
                mimeType = finalMime,
                totalBytes = finalOutFile.length()
            )
        } finally {
            workDir.deleteRecursively()
        }
    }

    fun translateSampleFlags(extractorFlags: Int): Int =
        if ((extractorFlags and MediaExtractor.SAMPLE_FLAG_SYNC) != 0) {
            MediaCodec.BUFFER_FLAG_KEY_FRAME
        } else {
            0
        }

    fun normalizeTrackPtsUs(rawPtsUs: Long, basePtsUs: Long, lastTrackPtsUs: Long): Long {
        val shifted = (rawPtsUs - basePtsUs).coerceAtLeast(0L)
        return if (lastTrackPtsUs >= 0L && shifted <= lastTrackPtsUs) {
            lastTrackPtsUs + 1L
        } else {
            shifted
        }
    }

    private fun remuxToStandardMp4(inputFile: File, outMp4: File): Boolean {
        val extractor = MediaExtractor()
        var muxer: MediaMuxer? = null
        try {
            extractor.setDataSource(inputFile.absolutePath)
            val trackCount = extractor.trackCount
            if (trackCount <= 0) return false

            muxer = MediaMuxer(outMp4.absolutePath, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)
            val trackMap = mutableMapOf<Int, Int>()
            val basePtsMap = mutableMapOf<Int, Long>()
            val lastPtsMap = mutableMapOf<Int, Long>()
            var maxBuf = 1024 * 1024

            for (i in 0 until trackCount) {
                val fmt = extractor.getTrackFormat(i)
                val mime = fmt.getString(MediaFormat.KEY_MIME) ?: continue
                if (mime.startsWith("video/") || mime.startsWith("audio/")) {
                    val dstIdx = muxer.addTrack(fmt)
                    trackMap[i] = dstIdx
                    extractor.selectTrack(i)
                    if (fmt.containsKey(MediaFormat.KEY_MAX_INPUT_SIZE)) {
                        maxBuf = maxOf(maxBuf, fmt.getInteger(MediaFormat.KEY_MAX_INPUT_SIZE))
                    }
                }
            }

            if (trackMap.isEmpty()) return false
            muxer.start()

            var buffer = ByteBuffer.allocate(maxBuf)
            val info = MediaCodec.BufferInfo()
            while (true) {
                val sampleSize = extractor.sampleSize
                if (sampleSize > buffer.capacity().toLong()) {
                    buffer = ByteBuffer.allocate((sampleSize.toInt() + 65536).coerceAtLeast(maxBuf))
                }
                buffer.clear()
                info.offset = 0
                info.size = extractor.readSampleData(buffer, 0)
                if (info.size < 0) break

                val srcIdx = extractor.sampleTrackIndex
                val dstIdx = trackMap[srcIdx]
                if (dstIdx != null) {
                    val rawPts = extractor.sampleTime
                    val basePts = basePtsMap.getOrPut(srcIdx) { rawPts.coerceAtLeast(0L) }
                    val lastPts = lastPtsMap[srcIdx] ?: -1L
                    val cleanPts = if (rawPts < 0L) {
                        (lastPts + 1L).coerceAtLeast(0L)
                    } else {
                        normalizeTrackPtsUs(rawPts, basePts, lastPts)
                    }
                    lastPtsMap[srcIdx] = cleanPts

                    info.presentationTimeUs = cleanPts
                    info.flags = translateSampleFlags(extractor.sampleFlags)
                    muxer.writeSampleData(dstIdx, buffer, info)
                }
                extractor.advance()
            }
            muxer.stop()
            return outMp4.exists() && outMp4.length() > 1024L
        } catch (_: Exception) {
            return false
        } finally {
            try {
                extractor.release()
            } catch (_: Exception) {}
            try {
                muxer?.release()
            } catch (_: Exception) {}
        }
    }

    private fun resolveRef(baseUri: URI, ref: String): String {
        val trimmed = ref.trim()
        val resolved = if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) {
            trimmed
        } else {
            baseUri.resolve(trimmed).toString()
        }
        // Preserve access_token query param on relative iw01.xyz references (e.g. master.m3u8 -> variant.m3u8)
        val baseQuery = baseUri.rawQuery
        if (!trimmed.startsWith("http://") && !trimmed.startsWith("https://") &&
            !resolved.contains("access_token=") &&
            baseQuery != null && baseQuery.contains("access_token=") &&
            resolved.contains("iw01.xyz", ignoreCase = true)
        ) {
            val tokenParam = baseQuery.split('&').firstOrNull { it.startsWith("access_token=") }
            if (tokenParam != null) {
                return resolved + (if (resolved.contains('?')) "&" else "?") + tokenParam
            }
        }
        return resolved
    }

    private fun parseHexIv(hex: String): ByteArray {
        val clean = hex.trim().padStart(32, '0').takeLast(32)
        val out = ByteArray(16)
        for (i in 0 until 16) {
            out[i] = clean.substring(i * 2, i * 2 + 2).toInt(16).toByte()
        }
        return out
    }

    private fun decryptIfNeeded(raw: ByteArray, key: ByteArray?, explicitIv: ByteArray?, seq: Long): ByteArray {
        if (key == null) return raw
        val iv = explicitIv ?: ByteArray(16).apply {
            var v = seq
            for (i in 15 downTo 8) {
                this[i] = (v and 0xffL).toByte()
                v = v ushr 8
            }
        }
        val cipher = Cipher.getInstance("AES/CBC/PKCS5Padding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), IvParameterSpec(iv))
        return cipher.doFinal(raw)
    }

    private fun fetchTextWithRetry(url: String, referer: String): String {
        val bytes = fetchBytesWithRetry(url, referer)
        return String(bytes, Charsets.UTF_8)
    }

    private fun fetchBytesWithRetry(url: String, referer: String): ByteArray {
        var lastErr: Exception? = null
        val cookieHeader = try {
            CookieManager.getInstance().getCookie(url)
        } catch (_: Exception) {
            null
        }
        val originHeader = try {
            val u = URI.create(referer)
            if (!u.scheme.isNullOrBlank() && !u.host.isNullOrBlank()) "${u.scheme}://${u.host}" else null
        } catch (_: Exception) {
            null
        }

        for (attempt in 1..3) {
            try {
                val builder = Request.Builder()
                    .url(url)
                    .header("User-Agent", DEFAULT_UA)
                    .header("Referer", referer)
                    .header("Accept", "*/*")
                if (!originHeader.isNullOrBlank()) builder.header("Origin", originHeader)
                if (!cookieHeader.isNullOrBlank()) builder.header("Cookie", cookieHeader)

                RetrofitClient.streamHttpClient.newCall(builder.build()).execute().use { resp ->
                    if (!resp.isSuccessful) {
                        throw IllegalStateException("HTTP ${resp.code} fetching segment/manifest")
                    }
                    return resp.body?.bytes() ?: throw IllegalStateException("Empty HTTP body")
                }
            } catch (e: Exception) {
                lastErr = e
                if (attempt < 3) Thread.sleep(350L * attempt)
            }
        }
        throw lastErr ?: IllegalStateException("Failed fetching $url")
    }

    private fun copyInto(src: File, out: FileOutputStream) {
        val buf = ByteArray(64 * 1024)
        FileInputStream(src).use { input ->
            while (true) {
                val n = input.read(buf)
                if (n <= 0) break
                out.write(buf, 0, n)
            }
        }
    }
}
