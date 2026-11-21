# KV Download — Product Context

## Purpose
KV Download is a self-hosted media extraction, batch download manager, in-app stream sniffer browser, and organized media library with a native Android companion app. Users run the backend on a Synology NAS, Docker host, or Linux workstation (`dl.khoavo.myds.me` or LAN IP `:9292`), and operate it across desktop browsers, mobile web, and native Android.

## Primary Users & Jobs
- **Power Collectors**: Batch-download entire YouTube channels/playlists, TikTok creator feeds, Facebook reels/videos, and SoundCloud albums into clean, deduplicated folders (`videos/<Channel>/<Title>.mp4`), monitor live transfers, retry failed items as a group, and download ZIP archives or sync directly to Android phone storage.
- **Stream Hunters**: Browse media-rich websites in the In-App Browser (including ISP-DNS-bypassing proxy mode) with live stream sniffing (`.m3u8`, `.mp4`, embedded players), preview streams before committing, and download individual or selected batches.
- **Mobile Viewers**: Use the native Android Jetpack Compose app to paste or share links straight from YouTube/TikTok/Facebook/Chrome, inspect grouped channel downloads, browse the server library or on-device storage, and save videos/audio/photos directly into phone `Movies/KV Download/`, `Music/KV Download/`, and `Pictures/KV Download/` folders.

## Surface Mode
- **Operate**: Scanability, crisp information density, explicit state feedback, keyboard/touch precision, and zero visual clutter. Both Light Mode and Dark Mode are first-class, accessible, and strictly tokenized through the Astryx Design System (`@astryxdesign/core`).
