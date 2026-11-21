package com.kvdownload.app.api

import android.net.Uri
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.logging.HttpLoggingInterceptor
import org.json.JSONObject
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.net.InetAddress
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
            return try {
                val req = Request.Builder()
                    .url("https://1.1.1.1/dns-query?name=${Uri.encode(hostname)}&type=A")
                    .header("Accept", "application/dns-json")
                    .build()
                dohBootstrapClient.newCall(req).execute().use { resp ->
                    if (!resp.isSuccessful) return null
                    val body = resp.body?.string() ?: return null
                    val json = JSONObject(body)
                    val answers = json.optJSONArray("Answer") ?: return null
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
            } catch (_: Exception) {
                null
            }
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

    fun downloadTaskUrl(taskId: String, inline: Boolean = false): String {
        val base = getBaseUrl()
        val encoded = Uri.encode(taskId)
        return if (inline) "$base/download?id=$encoded&inline=true" else "$base/download?id=$encoded"
    }

    fun downloadGalleryUrl(path: String, inline: Boolean = false): String {
        val base = getBaseUrl()
        val encoded = Uri.encode(path)
        return if (inline) "$base/download?file=$encoded&inline=true" else "$base/download?file=$encoded"
    }

    fun groupZipUrl(groupId: String): String =
        "${getBaseUrl()}/download/zip?groupId=${Uri.encode(groupId)}"

    fun folderZipUrl(folder: String): String =
        "${getBaseUrl()}/download/zip?folder=${Uri.encode(folder)}"

    fun taskThumbnailUrl(task: DownloadTask): String {
        val raw = task.thumbnail
        if (!raw.isNullOrBlank()) {
            return if (raw.startsWith("http://") || raw.startsWith("https://")) raw
            else "${getBaseUrl()}$raw"
        }
        return "${getBaseUrl()}/thumbnail?id=${Uri.encode(task.id)}"
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
        return "${getBaseUrl()}/thumbnail?path=${Uri.encode(item.path)}"
    }

    fun browserProxyUrl(targetUrl: String): String =
        "${getBaseUrl()}/api/browser/proxy?url=${Uri.encode(targetUrl)}"

    fun browserStreamUrl(mediaUrl: String): String {
        if (mediaUrl.startsWith(getBaseUrl())) return mediaUrl
        return if (mediaUrl.startsWith("/")) "${getBaseUrl()}$mediaUrl"
        else "${getBaseUrl()}/api/browser/stream?url=${Uri.encode(mediaUrl)}"
    }
}
