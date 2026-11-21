import type { Metadata, Viewport } from 'next';
import './layers.css';
import './globals.css';
import { PlayerProvider } from '@/context/PlayerContext';

export const metadata: Metadata = {
  title: 'KV Download — High Speed Media Downloader',
  description: 'Next-generation dark-studio multi-platform media downloader, stream sniffer and gallery player.',
  manifest: '/manifest.json',
};

export const viewport: Viewport = {
  themeColor: '#6d28d9',
  width: 'device-width',
  initialScale: 1,
  viewportFit: 'cover',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark" data-theme="dark" suppressHydrationWarning>
      <head>
        <meta name="color-scheme" content="light dark" />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          href="https://fonts.googleapis.com/css2?family=Hanken+Grotesk:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="antialiased selection:bg-violet-500/25 selection:text-inherit min-h-screen">
        <PlayerProvider>
          {children}
        </PlayerProvider>
      </body>
    </html>
  );
}
