package com.kvdownload.app.storage

import android.webkit.CookieManager
import com.kvdownload.app.api.RetrofitClient
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import okhttp3.Request
import okhttp3.Response
import java.io.File
import java.io.FileOutputStream
import java.io.RandomAccessFile
import java.net.URI
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.coroutineContext

data class ByteChunk(
    val index: Int,
    val start: Long,
    val endInclusive: Long
) {
    val size: Long get() = endInclusive - start + 1L
}

data class DirectDownloadResult(
    val finalUrl: String,
    val contentType: String?,
    val contentDisposition: String?,
    val totalBytes: Long,
    val usedParallelConnections: Int
)

/**
 * Thread-safe sliding-window speed meter shared by direct and HLS downloaders.
 */
class ThroughputMeter {
    private val lastSampleTimeNs = AtomicLong(System.nanoTime())
    private val lastSampleBytes = AtomicLong(0L)

    @Volatile
    private var smoothedBps: Double = 0.0

    @Synchronized
    fun sample(totalBytesNow: Long): Long {
        val now = System.nanoTime()
        val elapsedNs = now - lastSampleTimeNs.get()
        if (elapsedNs >= 350_000_000L) {
            val deltaBytes = (totalBytesNow - lastSampleBytes.get()).coerceAtLeast(0L)
            val instBps = (deltaBytes * 1_000_000_000.0) / elapsedNs.toDouble()
            smoothedBps = if (smoothedBps <= 0.0) instBps else smoothedBps * 0.45 + instBps * 0.55
            lastSampleTimeNs.set(now)
            lastSampleBytes.set(totalBytesNow)
        }
        return smoothedBps.toLong().coerceAtLeast(0L)
    }
}

object ParallelRangeDownloader {

    const val MAX_RANGE_WORKERS = 6
    const val MIN_MULTIPART_BYTES = 4L * 1024L * 1024L
    private const val MIN_CHUNK_BYTES = 2L * 1024L * 1024L
    private const val MAX_CHUNK_BYTES = 8L * 1024L * 1024L
    private const val IO_BUFFER_BYTES = 256 * 1024
    private const val MAX_CHUNK_RETRIES = 3

    fun planChunks(totalBytes: Long, maxWorkers: Int = MAX_RANGE_WORKERS): List<ByteChunk> {
        if (totalBytes <= 0L) return emptyList()
        if (totalBytes < MIN_MULTIPART_BYTES || maxWorkers <= 1) {
            return listOf(ByteChunk(index = 0, start = 0L, endInclusive = totalBytes - 1L))
        }
        // Keep roughly 60-120 balanced chunks so work-stealing tails never bottleneck on one slow socket.
        val targetChunk = (totalBytes / 80L).coerceIn(MIN_CHUNK_BYTES, MAX_CHUNK_BYTES)
        val out = ArrayList<ByteChunk>(((totalBytes / targetChunk) + 1).toInt())
        var offset = 0L
        var idx = 0
        while (offset < totalBytes) {
            val end = (offset + targetChunk - 1L).coerceAtMost(totalBytes - 1L)
            out.add(ByteChunk(index = idx, start = offset, endInclusive = end))
            offset = end + 1L
            idx++
        }
        return out
    }

    private fun buildRequest(url: String, referer: String?, rangeHeader: String? = null, isHead: Boolean = false): Request {
        val b = Request.Builder().url(url)
        if (isHead) b.head()
        if (!rangeHeader.isNullOrBlank()) b.header("Range", rangeHeader)
        b.header("Accept", "*/*")
        if (!referer.isNullOrBlank()) {
            b.header("Referer", referer)
            try {
                val u = URI.create(referer)
                if (!u.scheme.isNullOrBlank() && !u.host.isNullOrBlank()) {
                    b.header("Origin", "${u.scheme}://${u.host}")
                }
            } catch (_: Exception) {}
        }
        try {
            CookieManager.getInstance().getCookie(url)?.takeIf { it.isNotBlank() }?.let {
                b.header("Cookie", it)
            }
        } catch (_: Exception) {}
        return b.build()
    }

    private data class HeadProbeResult(
        val resolvedUrl: String,
        val contentLength: Long,
        val supportsRange: Boolean,
        val contentType: String?,
        val contentDisposition: String?
    )

    private fun probeHeadWithFallback(
        sourceUrl: String,
        fallbackUrl: String?,
        referer: String?
    ): HeadProbeResult? {
        val candidates = listOfNotNull(
            sourceUrl,
            fallbackUrl?.takeIf { it.isNotBlank() && it != sourceUrl }
        )
        for (candidate in candidates) {
            try {
                val hit = RetrofitClient.streamHttpClient.newCall(
                    buildRequest(candidate, referer, isHead = true)
                ).execute().use { resp ->
                    val ct = resp.header("Content-Type")
                    val lowerCt = ct?.lowercase().orEmpty()
                    if (!resp.isSuccessful || lowerCt.contains("text/html") || lowerCt.contains("application/xhtml")) {
                        null
                    } else {
                        val cl = resp.header("Content-Length")?.trim()?.toLongOrNull() ?: -1L
                        val acceptRanges = resp.header("Accept-Ranges")?.lowercase().orEmpty()
                        HeadProbeResult(
                            resolvedUrl = candidate,
                            contentLength = cl,
                            supportsRange = acceptRanges.contains("bytes"),
                            contentType = ct,
                            contentDisposition = resp.header("Content-Disposition")
                        )
                    }
                }
                if (hit != null) return hit
            } catch (_: Exception) {
                // Fall back to GET below.
            }
        }
        return null
    }

    suspend fun downloadDirectToPartFile(
        sourceUrl: String,
        fallbackSourceUrl: String?,
        referer: String?,
        partFile: File,
        allowMultipart: Boolean = true,
        onProgress: (downloadedBytes: Long, totalBytes: Long, speedBytesPerSec: Long) -> Unit
    ): DirectDownloadResult = withContext(Dispatchers.IO) {
        val canProbeRange = allowMultipart &&
            !sourceUrl.contains("/download/zip", ignoreCase = true) &&
            !sourceUrl.contains("compat=1", ignoreCase = true)

        val probe = if (canProbeRange) {
            probeHeadWithFallback(sourceUrl, fallbackSourceUrl, referer)
        } else {
            null
        }

        if (probe != null && probe.supportsRange && probe.contentLength >= MIN_MULTIPART_BYTES) {
            val parallelResult = runCatching {
                downloadMultipart(
                    url = probe.resolvedUrl,
                    referer = referer,
                    totalBytes = probe.contentLength,
                    contentType = probe.contentType,
                    contentDisposition = probe.contentDisposition,
                    partFile = partFile,
                    onProgress = onProgress
                )
            }
            if (parallelResult.isSuccess) {
                return@withContext parallelResult.getOrThrow()
            }
            val err = parallelResult.exceptionOrNull()
            if (err is CancellationException) throw err
            // Fall through cleanly to reliable single-stream GET if parallel Range fails on a quirky CDN.
            partFile.delete()
        }

        downloadSingleStream(
            sourceUrl = probe?.resolvedUrl ?: sourceUrl,
            fallbackSourceUrl = fallbackSourceUrl,
            referer = referer,
            partFile = partFile,
            onProgress = onProgress
        )
    }

    private suspend fun downloadMultipart(
        url: String,
        referer: String?,
        totalBytes: Long,
        contentType: String?,
        contentDisposition: String?,
        partFile: File,
        onProgress: (Long, Long, Long) -> Unit
    ): DirectDownloadResult {
        val chunks = planChunks(totalBytes, MAX_RANGE_WORKERS)
        val workerCount = minOf(MAX_RANGE_WORKERS, chunks.size)
        val nextChunkIdx = AtomicInteger(0)
        val downloaded = AtomicLong(0L)
        val meter = ThroughputMeter()
        val lastEmitMs = AtomicLong(0L)
        val rangeConfirmed = CompletableDeferred<Boolean>()

        RandomAccessFile(partFile, "rw").use { raf ->
            raf.setLength(totalBytes)
        }

        fun maybeEmitProgress(force: Boolean = false) {
            val now = System.currentTimeMillis()
            val prev = lastEmitMs.get()
            if (force || (now - prev >= 180L && lastEmitMs.compareAndSet(prev, now))) {
                val cur = downloaded.get().coerceAtMost(totalBytes)
                onProgress(cur, totalBytes, meter.sample(cur))
            }
        }

        coroutineScope {
            val jobs = (0 until workerCount).map { workerIdx ->
                async(Dispatchers.IO) {
                    if (workerIdx > 0) {
                        // Wait until Worker 0 confirms HTTP 206 Partial Content before spawning sibling streams.
                        if (!rangeConfirmed.await()) return@async
                    }

                    RandomAccessFile(partFile, "rw").use { raf ->
                        val ioBuf = ByteArray(IO_BUFFER_BYTES)
                        while (true) {
                            coroutineContext.ensureActive()
                            val idx = nextChunkIdx.getAndIncrement()
                            if (idx >= chunks.size) break
                            val chunk = chunks[idx]

                            var doneChunk = false
                            var lastErr: Exception? = null
                            for (attempt in 1..MAX_CHUNK_RETRIES) {
                                coroutineContext.ensureActive()
                                val call = RetrofitClient.streamHttpClient.newCall(
                                    buildRequest(url, referer, rangeHeader = "bytes=${chunk.start}-${chunk.endInclusive}")
                                )
                                try {
                                    call.execute().use { resp ->
                                        if (idx == 0 && resp.code == 200) {
                                            // Origin ignored Range header; stream complete file once sequentially.
                                            rangeConfirmed.complete(false)
                                            raf.setLength(0L)
                                            raf.seek(0L)
                                            val stream = resp.body?.byteStream()
                                                ?: throw IllegalStateException("Empty download body")
                                            var totalSeq = 0L
                                            while (true) {
                                                coroutineContext.ensureActive()
                                                val n = stream.read(ioBuf)
                                                if (n == -1) break
                                                raf.write(ioBuf, 0, n)
                                                totalSeq += n
                                                downloaded.set(totalSeq)
                                                maybeEmitProgress()
                                            }
                                            maybeEmitProgress(force = true)
                                            doneChunk = true
                                            nextChunkIdx.set(chunks.size)
                                            return@use
                                        }

                                        if (resp.code != 206) {
                                            throw IllegalStateException("Expected 206 Partial Content, got HTTP ${resp.code} on chunk $idx")
                                        }
                                        rangeConfirmed.complete(true)
                                        val bodyStream = resp.body?.byteStream()
                                            ?: throw IllegalStateException("Empty chunk body")
                                        var pos = chunk.start
                                        var chunkRead = 0L
                                        raf.seek(pos)
                                        while (true) {
                                            coroutineContext.ensureActive()
                                            val n = bodyStream.read(ioBuf)
                                            if (n == -1) break
                                            val allowed = minOf(n.toLong(), chunk.size - chunkRead).toInt()
                                            if (allowed <= 0) break
                                            raf.write(ioBuf, 0, allowed)
                                            pos += allowed
                                            chunkRead += allowed
                                            downloaded.addAndGet(allowed.toLong())
                                            maybeEmitProgress()
                                        }
                                        if (chunkRead < chunk.size) {
                                            // Rewind byte counter before retry.
                                            downloaded.addAndGet(-chunkRead)
                                            throw IllegalStateException("Truncated chunk $idx ($chunkRead/${chunk.size})")
                                        }
                                        doneChunk = true
                                    }
                                    if (doneChunk) break
                                } catch (e: CancellationException) {
                                    call.cancel()
                                    rangeConfirmed.completeExceptionally(e)
                                    throw e
                                } catch (e: Exception) {
                                    lastErr = e
                                    if (attempt < MAX_CHUNK_RETRIES) {
                                        delay(300L * attempt)
                                    }
                                }
                            }
                            if (!doneChunk) {
                                val err = lastErr ?: IllegalStateException("Chunk $idx failed")
                                rangeConfirmed.completeExceptionally(err)
                                throw err
                            }
                        }
                    }
                }
            }
            jobs.awaitAll()
        }

        val finalLen = partFile.length()
        if (finalLen <= 0L) throw IllegalStateException("Downloaded file is empty")
        maybeEmitProgress(force = true)
        return DirectDownloadResult(
            finalUrl = url,
            contentType = contentType,
            contentDisposition = contentDisposition,
            totalBytes = finalLen,
            usedParallelConnections = if (rangeConfirmed.isCompleted && runCatching { rangeConfirmed.getCompleted() }.getOrNull() == true) workerCount else 1
        )
    }

    private suspend fun downloadSingleStream(
        sourceUrl: String,
        fallbackSourceUrl: String?,
        referer: String?,
        partFile: File,
        onProgress: (Long, Long, Long) -> Unit
    ): DirectDownloadResult {
        var activeUrl = sourceUrl
        var response = try {
            RetrofitClient.streamHttpClient.newCall(buildRequest(activeUrl, referer)).execute()
        } catch (e: Exception) {
            if (!fallbackSourceUrl.isNullOrBlank() && fallbackSourceUrl != activeUrl) {
                activeUrl = fallbackSourceUrl
                RetrofitClient.streamHttpClient.newCall(buildRequest(activeUrl, referer)).execute()
            } else {
                throw e
            }
        }

        if (!response.isSuccessful && !fallbackSourceUrl.isNullOrBlank() && fallbackSourceUrl != activeUrl) {
            response.close()
            activeUrl = fallbackSourceUrl
            response = RetrofitClient.streamHttpClient.newCall(buildRequest(activeUrl, referer)).execute()
        }

        var initialCt = response.header("Content-Type")?.lowercase().orEmpty()
        if ((initialCt.contains("text/html") || initialCt.contains("application/xhtml")) &&
            !fallbackSourceUrl.isNullOrBlank() &&
            fallbackSourceUrl != activeUrl
        ) {
            response.close()
            activeUrl = fallbackSourceUrl
            response = RetrofitClient.streamHttpClient.newCall(buildRequest(activeUrl, referer)).execute()
            initialCt = response.header("Content-Type")?.lowercase().orEmpty()
        }

        return response.use { resp: Response ->
            if (!resp.isSuccessful) {
                throw IllegalStateException("HTTP ${resp.code}: ${resp.message}")
            }
            val ct = initialCt.ifBlank { resp.header("Content-Type")?.lowercase().orEmpty() }
            if (ct.contains("text/html") || ct.contains("application/xhtml")) {
                throw IllegalStateException("Target returned webpage instead of playable media")
            }
            val body = resp.body ?: throw IllegalStateException("Empty download body")
            val totalBytes = body.contentLength()
            val disp = resp.header("Content-Disposition")
            val contentTypeHeader = resp.header("Content-Type")

            // Note: If the server returns an HLS playlist directly here, caller checks rawCt / returns cleanly.
            if (ct.contains("mpegurl")) {
                return@use DirectDownloadResult(
                    finalUrl = activeUrl,
                    contentType = contentTypeHeader,
                    contentDisposition = disp,
                    totalBytes = -1L,
                    usedParallelConnections = 0
                )
            }

            val meter = ThroughputMeter()
            var written = 0L
            var lastEmit = 0L
            val buf = ByteArray(IO_BUFFER_BYTES)
            FileOutputStream(partFile).use { out ->
                body.byteStream().use { input ->
                    while (true) {
                        coroutineContext.ensureActive()
                        val n = input.read(buf)
                        if (n == -1) break
                        out.write(buf, 0, n)
                        written += n
                        val now = System.currentTimeMillis()
                        if (now - lastEmit >= 180L) {
                            onProgress(written, totalBytes, meter.sample(written))
                            lastEmit = now
                        }
                    }
                }
                out.flush()
            }
            if (written <= 0L) throw IllegalStateException("Downloaded file is empty")
            onProgress(written, if (totalBytes > 0L) totalBytes else written, meter.sample(written))
            DirectDownloadResult(
                finalUrl = activeUrl,
                contentType = contentTypeHeader,
                contentDisposition = disp,
                totalBytes = written,
                usedParallelConnections = 1
            )
        }
    }
}
