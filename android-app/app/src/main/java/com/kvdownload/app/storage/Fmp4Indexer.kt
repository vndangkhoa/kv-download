package com.kvdownload.app.storage

import java.io.File
import java.io.RandomAccessFile
import java.nio.ByteBuffer

/**
 * Fragmented MP4 files written by merging HLS fMP4 segments have no `sidx` index, so players
 * (ExoPlayer included) treat them as unseekable and every seek jumps back to 0:00.
 * [addSidxIfMissing] inserts a version-1 `sidx` box right after `moov` that lists every
 * moof+mdat fragment, which makes the file seekable without re-encoding.
 */
object Fmp4Indexer {

    private class Box(val type: String, val offset: Long, val size: Long)

    private class Child(val type: String, val payload: Int, val end: Int)

    data class Fragment(val offset: Long, val size: Long, val baseDecodeTime: Long)

    private fun u32(b: ByteArray, p: Int): Long =
        ((b[p].toLong() and 0xFF) shl 24) or ((b[p + 1].toLong() and 0xFF) shl 16) or
            ((b[p + 2].toLong() and 0xFF) shl 8) or (b[p + 3].toLong() and 0xFF)

    private fun u64(b: ByteArray, p: Int): Long = (u32(b, p) shl 32) or u32(b, p + 4)

    private fun children(buf: ByteArray, start: Int, end: Int): List<Child> {
        val out = ArrayList<Child>()
        var p = start
        while (p + 8 <= end) {
            var size = u32(buf, p)
            val type = String(buf, p + 4, 4, Charsets.ISO_8859_1)
            var header = 8
            if (size == 1L && p + 16 <= end) {
                size = u64(buf, p + 8)
                header = 16
            } else if (size == 0L) {
                size = (end - p).toLong()
            }
            if (size < header || p + size > end) break
            out.add(Child(type, p + header, (p + size).toInt()))
            p += size.toInt()
        }
        return out
    }

    private fun readTopLevel(raf: RandomAccessFile, length: Long): List<Box> {
        val boxes = ArrayList<Box>()
        val head = ByteArray(16)
        var offset = 0L
        while (offset + 8 <= length) {
            raf.seek(offset)
            raf.readFully(head, 0, 8)
            var size = u32(head, 0)
            val type = String(head, 4, 4, Charsets.ISO_8859_1)
            if (size == 1L) {
                if (offset + 16 > length) break
                raf.readFully(head, 8, 8)
                size = u64(head, 8)
            } else if (size == 0L) {
                size = length - offset
            }
            if (size < 8) break
            if (offset + size > length) size = length - offset
            boxes.add(Box(type, offset, size))
            offset += size
        }
        return boxes
    }

    /** Returns (trackId, timescale) of the first video track (or first track as fallback). */
    private fun pickTrack(moov: ByteArray): Pair<Long, Long>? {
        var firstAny: Pair<Long, Long>? = null
        for (trak in children(moov, 0, moov.size).flatMap { top ->
            if (top.type == "moov") children(moov, top.payload, top.end) else listOf(top)
        }.filter { it.type == "trak" }) {
            val kids = children(moov, trak.payload, trak.end)
            val tkhd = kids.firstOrNull { it.type == "tkhd" } ?: continue
            val mdia = kids.firstOrNull { it.type == "mdia" } ?: continue
            val mdKids = children(moov, mdia.payload, mdia.end)
            val mdhd = mdKids.firstOrNull { it.type == "mdhd" } ?: continue
            val hdlr = mdKids.firstOrNull { it.type == "hdlr" }
            val tkV1 = moov[tkhd.payload].toInt() == 1
            val trackId = if (tkV1) u32(moov, tkhd.payload + 20) else u32(moov, tkhd.payload + 12)
            val mdV1 = moov[mdhd.payload].toInt() == 1
            val timescale = if (mdV1) u32(moov, mdhd.payload + 20) else u32(moov, mdhd.payload + 12)
            if (timescale <= 0L) continue
            val pair = trackId to timescale
            if (firstAny == null) firstAny = pair
            val handler = hdlr?.let { String(moov, it.payload + 8, 4, Charsets.ISO_8859_1) }
            if (handler == "vide") return pair
        }
        return firstAny
    }

    /** base_media_decode_time of the given track inside a moof payload, or -1. */
    private fun baseDecodeTime(moof: ByteArray, trackId: Long): Long {
        for (traf in children(moof, 8, moof.size).filter { it.type == "traf" }) {
            val kids = children(moof, traf.payload, traf.end)
            val tfhd = kids.firstOrNull { it.type == "tfhd" } ?: continue
            if (u32(moof, tfhd.payload + 4) != trackId) continue
            val tfdt = kids.firstOrNull { it.type == "tfdt" } ?: return -1L
            return if (moof[tfdt.payload].toInt() == 1) u64(moof, tfdt.payload + 4) else u32(moof, tfdt.payload + 4)
        }
        return -1L
    }

    fun hasFragmentsWithoutIndex(file: File): Boolean = try {
        RandomAccessFile(file, "r").use { raf ->
            val boxes = readTopLevel(raf, raf.length())
            boxes.any { it.type == "moof" } && boxes.none { it.type == "sidx" } && boxes.any { it.type == "moov" }
        }
    } catch (_: Exception) {
        false
    }

    /** Builds the sidx box bytes for the given fragments. */
    fun buildSidx(trackId: Long, timescale: Long, firstOffset: Long, fragments: List<Fragment>): ByteArray {
        val refCount = fragments.size
        val size = 12 + 4 + 4 + 8 + 8 + 4 + refCount * 12
        val bb = ByteBuffer.allocate(size)
        bb.putInt(size)
        bb.put("sidx".toByteArray(Charsets.ISO_8859_1))
        bb.putInt(0x01000000) // version 1, flags 0
        bb.putInt(trackId.toInt())
        bb.putInt(timescale.toInt())
        bb.putLong(fragments.first().baseDecodeTime)
        bb.putLong(firstOffset)
        bb.putShort(0)
        bb.putShort(refCount.toShort())
        for (i in fragments.indices) {
            val f = fragments[i]
            val dur = if (i + 1 < fragments.size) {
                fragments[i + 1].baseDecodeTime - f.baseDecodeTime
            } else if (i > 0) {
                f.baseDecodeTime - fragments[i - 1].baseDecodeTime
            } else {
                timescale * 6
            }
            bb.putInt((f.size and 0x7FFFFFFFL).toInt())
            bb.putInt(dur.coerceIn(1L, 0xFFFFFFFFL).toInt())
            bb.putInt(0x90000000.toInt()) // starts_with_SAP=1, SAP_type=1, delta=0
        }
        return bb.array()
    }

    /**
     * Inserts a `sidx` after `moov` when the file is fragmented and has none.
     * Returns true when the file is (now) indexed, false if it was left untouched.
     */
    fun addSidxIfMissing(file: File): Boolean {
        val tmp = File(file.parentFile, file.name + ".idx.tmp")
        try {
            var moovEnd = 0L
            var firstMoofOffset = 0L
            val fragments = ArrayList<Fragment>()
            var trackId = 0L
            var timescale = 0L
            val length = file.length()
            RandomAccessFile(file, "r").use { raf ->
                val boxes = readTopLevel(raf, length)
                if (boxes.any { it.type == "sidx" }) return true
                val moovBox = boxes.firstOrNull { it.type == "moov" } ?: return false
                val moofs = boxes.filter { it.type == "moof" }
                if (moofs.isEmpty() || moofs.first().offset < moovBox.offset + moovBox.size) return false
                if (moofs.size > 65_000 || moovBox.size > 64L * 1024 * 1024) return false

                val moov = ByteArray(moovBox.size.toInt())
                raf.seek(moovBox.offset)
                raf.readFully(moov)
                val track = pickTrack(moov) ?: return false
                trackId = track.first
                timescale = track.second

                moovEnd = moovBox.offset + moovBox.size
                firstMoofOffset = moofs.first().offset
                for ((i, m) in moofs.withIndex()) {
                    if (m.size > 16L * 1024 * 1024) return false
                    val moof = ByteArray(m.size.toInt())
                    raf.seek(m.offset)
                    raf.readFully(moof)
                    val bdt = baseDecodeTime(moof, trackId)
                    if (bdt < 0L) return false
                    val nextStart = if (i + 1 < moofs.size) moofs[i + 1].offset else length
                    fragments.add(Fragment(m.offset, nextStart - m.offset, bdt))
                }
            }
            val sidx = buildSidx(trackId, timescale, firstMoofOffset - moovEnd, fragments)
            RandomAccessFile(file, "r").use { src ->
                java.io.FileOutputStream(tmp).use { out ->
                    val buf = ByteArray(1024 * 1024)
                    fun copy(from: Long, to: Long) {
                        src.seek(from)
                        var left = to - from
                        while (left > 0) {
                            val n = src.read(buf, 0, minOf(buf.size.toLong(), left).toInt())
                            if (n <= 0) break
                            out.write(buf, 0, n)
                            left -= n
                        }
                    }
                    copy(0L, moovEnd)
                    out.write(sidx)
                    copy(moovEnd, length)
                    out.flush()
                }
            }
            if (!tmp.renameTo(file)) {
                tmp.copyTo(file, overwrite = true)
                tmp.delete()
            }
            return true
        } catch (_: Exception) {
            tmp.delete()
            return false
        }
    }
}
