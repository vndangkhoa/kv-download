import React, { useCallback, useState } from 'react';
import {
  Alert,
  Button,
  Linking,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { InAppBrowser } from 'react-native-inappbrowser-reborn';

const KV_BASE = 'http://10.0.2.2:9292'; // Android emulator -> host. For iOS simulator use http://localhost:9292. For real device use your LAN IP.

async function openWithInAppBrowser(url: string) {
  try {
    if (await InAppBrowser.isAvailable()) {
      const result = await InAppBrowser.open(url, {
        dismissButtonStyle: 'close',
        preferredBarTintColor: '#0a0b0d',
        preferredControlTintColor: '#d0bcff',
        readerMode: false,
        animated: true,
        modalPresentationStyle: 'fullScreen',
        modalTransitionStyle: 'coverVertical',
        modalEnabled: true,
        enableBarCollapsing: false,
        ephemeralWebSession: false,
        // Android
        showTitle: true,
        toolbarColor: '#0a0b0d',
        secondaryToolbarColor: '#121315',
        navigationBarColor: '#0a0b0d',
        navigationBarDividerColor: '#334155',
        enableUrlBarHiding: true,
        enableDefaultShare: true,
        forceCloseOnRedirection: false,
        animations: {
          startEnter: 'slide_in_right',
          startExit: 'slide_out_left',
          endEnter: 'slide_in_left',
          endExit: 'slide_out_right',
        },
      });
      return result;
    } else {
      return Linking.openURL(url);
    }
  } catch (e: any) {
    Alert.alert('InAppBrowser error', e?.message ?? String(e));
    return Linking.openURL(url);
  }
}

export default function App() {
  const [url, setUrl] = useState('https://www.google.com/search?q=hello');
  const [lastResult, setLastResult] = useState<string>('');

  const open = useCallback(async () => {
    const trimmed = url.trim();
    const target = trimmed.startsWith('http') ? trimmed : `https://${trimmed}`;
    const res: any = await openWithInAppBrowser(target);
    if (res) setLastResult(JSON.stringify(res, null, 2));
  }, [url]);

  const openKV = useCallback(async () => {
    const res: any = await openWithInAppBrowser(KV_BASE);
    if (res) setLastResult(JSON.stringify(res, null, 2));
  }, []);

  const openGoogle = useCallback(async () => {
    const res: any = await openWithInAppBrowser('https://www.google.com');
    if (res) setLastResult(JSON.stringify(res, null, 2));
  }, []);

  const openAuthExample = useCallback(async () => {
    // OAuth example — replace with your real auth URL + deep link
    const deepLink = 'kvdownload://callback';
    try {
      if (await InAppBrowser.isAvailable()) {
        const res: any = await InAppBrowser.openAuth(
          `https://example.com/oauth?redirect_uri=${encodeURIComponent(deepLink)}`,
          deepLink,
          {
            ephemeralWebSession: false,
            showTitle: false,
            enableUrlBarHiding: true,
            enableDefaultShare: false,
          }
        );
        if (res?.type === 'success' && res.url) Linking.openURL(res.url);
        setLastResult(JSON.stringify(res, null, 2));
      }
    } catch (e: any) {
      Alert.alert(e.message);
    }
  }, []);

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.container}>
        <Text style={styles.title}>KV Download — In-App Browser</Text>
        <Text style={styles.subtitle}>
          Powered by Chrome Custom Tabs (Android) & SFSafariViewController (iOS). Fixes
          google.com refused to connect — no iframe, system browser.
        </Text>

        <View style={styles.card}>
          <Text style={styles.label}>URL to open</Text>
          <TextInput
            value={url}
            onChangeText={setUrl}
            placeholder="https://www.google.com/search?q=hello"
            placeholderTextColor="#94a3b8"
            autoCapitalize="none"
            autoCorrect={false}
            style={styles.input}
          />
          <View style={styles.row}>
            <View style={styles.btn}><Button title="Open URL" onPress={open} /></View>
            <View style={styles.btn}><Button title="Open KV" onPress={openKV} /></View>
            <View style={styles.btn}><Button title="Google" onPress={openGoogle} /></View>
          </View>
          <View style={styles.row}>
            <View style={styles.btn}><Button title="OAuth demo (openAuth)" onPress={openAuthExample} /></View>
          </View>
        </View>

        <View style={styles.card}>
          <Text style={styles.label}>Setup</Text>
          <Text style={styles.mono}>npm install && npx react-native run-android</Text>
          <Text style={styles.mono}>cd ios && pod install && cd .. && npx react-native run-ios</Text>
          <Text style={styles.hint}>
            For real device, change KV_BASE in App.tsx to your LAN IP (e.g.
            http://192.168.1.10:9292). Android emulator uses 10.0.2.2.
          </Text>
        </View>

        {lastResult ? (
          <View style={styles.card}>
            <Text style={styles.label}>Last result</Text>
            <Text style={styles.mono}>{lastResult}</Text>
          </View>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: '#0a0b0d' },
  container: { padding: 16, gap: 16, paddingBottom: 40 },
  title: { color: '#fff', fontSize: 18, fontWeight: '700' },
  subtitle: { color: '#94a3b8', fontSize: 12, lineHeight: 16 },
  card: {
    backgroundColor: '#121315',
    borderColor: 'rgba(255,255,255,0.08)',
    borderWidth: 1,
    borderRadius: 16,
    padding: 14,
    gap: 10,
  },
  label: { color: '#e3e2e5', fontSize: 12, fontWeight: '600' },
  input: {
    backgroundColor: '#07080a',
    borderColor: 'rgba(255,255,255,0.1)',
    borderWidth: 1,
    borderRadius: 12,
    color: '#fff',
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 13,
  },
  row: { flexDirection: 'row', gap: 8, flexWrap: 'wrap' },
  btn: { flexGrow: 1 },
  mono: { color: '#d0bcff', fontFamily: 'monospace', fontSize: 11 },
  hint: { color: '#64748b', fontSize: 11 },
});
