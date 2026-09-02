# Deep Link — Web → Native handoff

Web `In-App Browser` (iframe proxy) now detects Google / system sites that would `refused to connect` and can hand off to the native `InAppBrowser`.

## Web side (already in `templates/media/index.html`)

```js
// For sites that block iframes (google, facebook login, etc.)
function isSystemSite(url) {
  return /google\.com|accounts\.google|facebook\.com\/login/i.test(url);
}
if (isSystemSite(url)) {
  window.open(url, '_blank'); // system browser — same as native InAppBrowser.open()
} else {
  iframe.src = '/api/browser/proxy?url=' + encodeURIComponent(url); // stays in picker + bubble
}
```
Bubble sniffer still works for proxied video pages.
```

## Native side

Already scaffolded: `App.tsx` uses `InAppBrowser.open(url, { toolbarColor: '#0a0b0d' })` for browsing and `openAuth(url, 'kvdownload://callback')` for OAuth.

### Enable `kvdownload://callback` deep link

**Android** — `mobile/android/app/src/main/AndroidManifest.xml`:

```xml
<activity android:name=".MainActivity" android:launchMode="singleTask">
  <intent-filter>
    <action android:name="android.intent.action.VIEW"/>
    <category android:name="android.intent.category.DEFAULT"/>
    <category android:name="android.intent.category.BROWSABLE"/>
    <data android:scheme="kvdownload" android:host="callback"/>
  </intent-filter>
</activity>
```

**iOS** — `mobile/ios/<App>/Info.plist`:

```xml
<key>CFBundleURLTypes</key>
<array>
  <dict>
    <key>CFBundleURLSchemes</key><array><string>kvdownload</string></array>
  </dict>
</array>
```

Then test: `npx uri-scheme open kvdownload://callback --android` / `--ios`
