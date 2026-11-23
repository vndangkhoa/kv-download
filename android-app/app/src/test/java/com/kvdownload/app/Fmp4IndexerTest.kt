package com.kvdownload.app

import com.kvdownload.app.storage.Fmp4Indexer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.io.File
import java.nio.ByteBuffer

class Fmp4IndexerTest {

    private fun box(type: String, payload: ByteArray): ByteArray {
        val bb = ByteBuffer.allocate(8 + payload.size)
        bb.putInt(8 + payload.size)
        bb.put(type.toByteArray(Charsets.ISO_8859_1))
        bb.put(payload)
        return bb.array()
    }

    private fun cat(vararg parts: ByteArray): ByteArray {
        val out = ByteArrayOutputStream()
        parts.forEach { out.write(it) }
        return out.toByteArray()
    }

    private fun tkhd(id: Int): ByteArray {
        val bb = ByteBuffer.allocate(4 + 4 + 4 + 4 + 4 + 60)
        bb.putInt(0); bb.putInt(0); bb.putInt(0); bb.putInt(id)
        return box("tkhd", bb.array())
    }

    private fun mdhd(timescale: Int): ByteArray {
        val bb = ByteBuffer.allocate(24)
        bb.putInt(0); bb.putInt(0); bb.putInt(0); bb.putInt(timescale); bb.putInt(0); bb.putInt(0)
        return box("mdhd", bb.array())
    }

    private fun hdlr(handler: String): ByteArray {
        val bb = ByteBuffer.allocate(4 + 4 + 4 + 12 + 1)
        bb.putInt(0); bb.putInt(0)
        bb.put(handler.toByteArray(Charsets.ISO_8859_1))
        return box("hdlr", bb.array())
    }

    private fun moov(): ByteArray {
        val audio = box("trak", cat(tkhd(2), box("mdia", cat(mdhd(44100), hdlr("soun")))))
        val video = box("trak", cat(tkhd(1), box("mdia", cat(mdhd(30000), hdlr("vide")))))
        return box("moov", cat(audio, video))
    }

    private fun moof(videoTfdt: Long, audioTfdt: Long): ByteArray {
        fun traf(id: Int, tfdt: Long): ByteArray {
            val tfhd = ByteBuffer.allocate(8).apply { putInt(0); putInt(id) }.array()
            val tfdtBytes = ByteBuffer.allocate(12).apply { putInt(0x01000000); putLong(tfdt) }.array()
            return box("traf", cat(box("tfhd", tfhd), box("tfdt", tfdtBytes)))
        }
        val mfhd = box("mfhd", ByteBuffer.allocate(8).apply { putInt(0); putInt(1) }.array())
        return box("moof", cat(mfhd, traf(2, audioTfdt), traf(1, videoTfdt)))
    }

    private fun fragmented(): ByteArray {
        val ftyp = box("ftyp", ByteArray(16))
        val mdat = box("mdat", ByteArray(100))
        return cat(
            ftyp, moov(),
            moof(0, 0), mdat,
            moof(270_000, 396_900), mdat,
            moof(540_000, 793_800), mdat
        )
    }

    @Test
    fun insertsSidxAfterMoovWithOneReferencePerFragment() {
        val dir = File(System.getProperty("java.io.tmpdir"), "kv_fmp4_" + System.nanoTime()).apply { mkdirs() }
        val f = File(dir, "a.mp4")
        val original = fragmented()
        f.writeBytes(original)
        assertTrue(Fmp4Indexer.hasFragmentsWithoutIndex(f))

        assertTrue(Fmp4Indexer.addSidxIfMissing(f))
        assertFalse(Fmp4Indexer.hasFragmentsWithoutIndex(f))

        val out = f.readBytes()
        val moovEnd = 24 + moov().size
        val bb = ByteBuffer.wrap(out)
        assertEquals("sidx", String(out, moovEnd + 4, 4, Charsets.ISO_8859_1))
        val sidxSize = bb.getInt(moovEnd)
        assertEquals(out.size, original.size + sidxSize)

        var p = moovEnd + 8
        assertEquals(0x01000000, bb.getInt(p)); p += 4
        assertEquals(1, bb.getInt(p)); p += 4 // video track id
        assertEquals(30000, bb.getInt(p)); p += 4 // video timescale
        assertEquals(0L, bb.getLong(p)); p += 8 // earliest presentation time
        assertEquals(0L, bb.getLong(p)); p += 8 // first offset
        p += 2
        assertEquals(3, bb.getShort(p).toInt()); p += 2

        val fragSize = moof(0, 0).size + box("mdat", ByteArray(100)).size
        for (i in 0 until 3) {
            assertEquals(fragSize, bb.getInt(p)); p += 4
            assertEquals(270_000, bb.getInt(p)); p += 4
            assertEquals(0x90000000.toInt(), bb.getInt(p)); p += 4
        }
        // everything after the sidx is the untouched original payload
        assertTrue(out.copyOfRange(moovEnd + sidxSize, out.size).contentEquals(original.copyOfRange(moovEnd, original.size)))
        dir.deleteRecursively()
    }

    @Test
    fun leavesProgressiveMp4Alone() {
        val dir = File(System.getProperty("java.io.tmpdir"), "kv_fmp4_" + System.nanoTime()).apply { mkdirs() }
        val f = File(dir, "p.mp4")
        f.writeBytes(cat(box("ftyp", ByteArray(16)), moov(), box("mdat", ByteArray(200))))
        assertFalse(Fmp4Indexer.hasFragmentsWithoutIndex(f))
        assertFalse(Fmp4Indexer.addSidxIfMissing(f))
        dir.deleteRecursively()
    }

    @Test
    fun indexesRealSampleWhenProvided() {
        val sample = System.getenv("KV_FMP4_SAMPLE") ?: return
        val outPath = System.getenv("KV_FMP4_OUT") ?: return
        val copy = File(outPath)
        File(sample).copyTo(copy, overwrite = true)
        assertTrue(Fmp4Indexer.addSidxIfMissing(copy))
    }
}
