package com.kvdownload.app.api

import android.net.Uri
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.logging.HttpLoggingInterceptor
import org.json.JSONObject
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.net.InetAddress
import java.net.URLEncoder
import java.net.UnknownHostException
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

object RetrofitClient {
    const val DEFAULT_SERVER_URL = "https://dl.khoavo.myds.me"

    @Volatile
    private var currentBaseUrl: String = DEFAULT_SERVER_URL

    @Volatile
    private var cachedApi: KVDownloadApi? = null

    private val dohCache = ConcurrentHashMap<String, Pair<List<InetAddress>, Long>>()

    private val dohBootstrapClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .readTimeout(5, TimeUnit.SECONDS)
            .build()
    }

    /**
     * System DNS with Cloudflare DoH (1.1.1.1) fallback for ISP-blocked hostnames
     * such as av01.media when accessed directly on mobile data or LAN routers.
     */
    private val resilientDns = object : Dns {
        override fun lookup(hostname: String): List<InetAddress> {
            try {
                val system = Dns.SYSTEM.lookup(hostname)
                if (system.isNotEmpty()) return system
            } catch (_: UnknownHostException) {
                // Fall back to DoH.
            }

            val cached = dohCache[hostname]
            val now = System.currentTimeMillis()
            if (cached != null && now - cached.second < 180_000L) {
                return cached.first
            }

            return resolveViaDoH(hostname)?.also {
                dohCache[hostname] = Pair(it, now)
            } ?: throw UnknownHostException("Unable to resolve $hostname via system DNS or DoH")
        }

        private fun resolveViaDoH(hostname: String): List<InetAddress>? {
            val endpoints = listOf(
                "https://1.1.1.1/dns-query?name=${Uri.encode(hostname)}&type=A",
                "https://dns.google/resolve?name=${Uri.encode(hostname)}&type=A"
            )
            for (endpoint in endpoints) {
                try {
                    val req = Request.Builder()
                        .url(endpoint)
                        .header("Accept", "application/dns-json")
                        .build()
                    val resolved = dohBootstrapClient.newCall(req).execute().use { resp ->
                        if (!resp.isSuccessful) {
                            null
                        } else {
                            val body = resp.body?.string()
                            if (body.isNullOrBlank()) {
                                null
                            } else {
                                val json = JSONObject(body)
                                val answers = json.optJSONArray("Answer")
                                if (answers == null) {
                                    null
                                } else {
                                    val out = mutableListOf<InetAddress>()
                                    for (i in 0 until answers.length()) {
                                        val ans = answers.getJSONObject(i)
                                        if (ans.optInt("type") == 1) {
                                            val ip = ans.optString("data")
                                            if (ip.isNotBlank()) {
                                                out.add(InetAddress.getByName(ip))
                                            }
                                        }
                                    }
                                    out.ifEmpty { null }
                                }
                            }
                        }
                    }
                    if (!resolved.isNullOrEmpty()) return resolved
                } catch (_: Exception) {
                }
            }
            return null
        }
    }

    val okHttpClient: OkHttpClient by lazy {
        val logger = HttpLoggingInterceptor().apply {
            level = HttpLoggingInterceptor.Level.BASIC
        }
        OkHttpClient.Builder()
            .dns(resilientDns)
            .connectTimeout(25, TimeUnit.SECONDS)
            .readTimeout(90, TimeUnit.SECONDS)
            .writeTimeout(60, TimeUnit.SECONDS)
            .addInterceptor(logger)
            .build()
    }

    val streamHttpClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .dns(resilientDns)
            .connectTimeout(30, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .followRedirects(true)
            .build()
    }

    /**
     * Bounded HTTP/1.1 client dedicated to ExoPlayer video/audio playback so Range
     * reads never stall indefinitely or hit HTTP/2 flow-control locks on Synology Nginx.
     */
    val playerHttpClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .dns(resilientDns)
            .protocols(listOf(Protocol.HTTP_1_1))
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .writeTimeout(15, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .followRedirects(true)
            .build()
    }

    @Synchronized
    fun configure(serverUrl: String) {
        val normalized = normalizeUrl(serverUrl)
        if (normalized != currentBaseUrl || cachedApi == null) {
            currentBaseUrl = normalized
            cachedApi = Retrofit.Builder()
                .baseUrl("$normalized/")
                .client(okHttpClient)
                .addConverterFactory(GsonConverterFactory.create())
                .build()
                .create(KVDownloadApi::class.java)
        }
    }

    fun getApi(): KVDownloadApi {
        return cachedApi ?: synchronized(this) {
            cachedApi ?: Retrofit.Builder()
                .baseUrl("${getBaseUrl()}/")
                .client(okHttpClient)
                .addConverterFactory(GsonConverterFactory.create())
                .build()
                .create(KVDownloadApi::class.java)
                .also { cachedApi = it }
        }
    }

    fun getBaseUrl(): String = currentBaseUrl

    fun normalizeUrl(raw: String): String {
        val trimmed = raw.trim().trimEnd('/')
        if (trimmed.isBlank()) return DEFAULT_SERVER_URL
        return if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) {
            trimmed
        } else {
            "https://$trimmed"
        }
    }

    private fun urlEncode(raw: String): String =
        URLEncoder.encode(raw, "UTF-8").replace("+", "%20")

    fun downloadTaskUrl(taskId: String, inline: Boolean = false, mediaId: String? = null): String {
        val base = getBaseUrl()
        val encoded = urlEncode(taskId)
        val mediaIdPart = if (!mediaId.isNullOrBlank()) "&mediaId=${urlEncode(mediaId)}" else ""
        return if (inline) {
            "$base/download?id=$encoded$mediaIdPart&inline=true"
        } else {
            "$base/download?id=$encoded$mediaIdPart"
        }
    }

    fun downloadGalleryUrl(path: String, inline: Boolean = false): String {
        val base = getBaseUrl()
        val encoded = urlEncode(path)
        return if (inline) "$base/download?file=$encoded&inline=true" else "$base/download?file=$encoded"
    }

    fun groupZipUrl(groupId: String): String =
        "${getBaseUrl()}/download/zip?groupId=${urlEncode(groupId)}"

    fun folderZipUrl(folder: String): String =
        "${getBaseUrl()}/download/zip?folder=${urlEncode(folder)}"

    fun taskThumbnailUrl(task: DownloadTask): String {
        val raw = task.thumbnail
        if (!raw.isNullOrBlank()) {
            return if (raw.startsWith("http://") || raw.startsWith("https://")) raw
            else "${getBaseUrl()}$raw"
        }
        val mediaIdPart = if (!task.mediaId.isNullOrBlank()) "&mediaId=${urlEncode(task.mediaId)}" else ""
        return "${getBaseUrl()}/thumbnail?id=${urlEncode(task.id)}$mediaIdPart"
    }

    fun galleryThumbnailUrl(item: GalleryItem): String {
        if (item.type == "photo") {
            return downloadGalleryUrl(item.path, inline = true)
        }
        val raw = item.thumbnail
        if (!raw.isNullOrBlank()) {
            return if (raw.startsWith("http://") || raw.startsWith("https://")) raw
            else "${getBaseUrl()}$raw"
        }
        return "${getBaseUrl()}/thumbnail?path=${urlEncode(item.path)}"
    }

    fun browserProxyUrl(targetUrl: String): String =
        "${getBaseUrl()}/api/browser/proxy?url=${urlEncode(targetUrl)}"

    fun browserStreamUrl(mediaUrl: String, referer: String? = null): String {
        if (mediaUrl.startsWith(getBaseUrl())) return mediaUrl
        if (mediaUrl.startsWith("/")) return "${getBaseUrl()}$mediaUrl"
        val base = "${getBaseUrl()}/api/browser/stream?url=${urlEncode(mediaUrl)}"
        return if (!referer.isNullOrBlank()) "$base&referer=${urlEncode(referer)}" else base
    }
}
