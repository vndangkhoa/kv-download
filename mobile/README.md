# KV Download — Mobile (Option 2: React Native InAppBrowser)

Wraps your Go backend at `http://localhost:9292` with **Chrome Custom Tabs (Android)** + **SFSafariViewController (iOS)** via `react-native-inappbrowser-reborn`. Fixes `google.com refused to connect` — no iframe, no X-Frame-Options block.

> Your current web `In-App Browser` uses a proxied `iframe` so it can sniff `m3u8` + overlay the download picker. The native `InAppBrowser` opens the **system browser** instead — perfect for Google/Search, but it cannot overlay a picker inside the page.

## Quick start

```bash
cd mobile
npm install
# iOS
cd ios && pod install && cd .. 
npx react-native run-ios
# Android
npx react-native run-android
```

- Android emulator → backend is `http://10.0.2.2:9292` (already set in `App.tsx`)
- iOS simulator → change `KV_BASE` to `http://localhost:9292`
- Real device → change `KV_BASE` to your LAN IP, e.g. `http://192.168.1.10:9292` (and `docker compose up -d` must bind `0.0.0.0:9292`)

## How it works

`App.tsx` → `InAppBrowser.open(url, { toolbarColor, ... })` for normal browsing, `InAppBrowser.openAuth(url, deepLink)` for OAuth redirects.

Deep links (for OAuth): configure `kvdownload://callback`:
- Android: `android/app/src/main/AndroidManifest.xml` intent-filter
- iOS: `Info.plist` CFBundleURLTypes

## Hybrid recommendation (keep current picker)

Keep the proxied iframe for video sites (so the bubble picker works), but deep-link Google/search to the system browser:

In your web `templates/media/index.html` navigation, do: `window.open('https://www.google.com/search?q=...', '_blank')` for Google, proxied iframe for everything else.

This mobile app is the native equivalent — same split, but via `InAppBrowser`.
