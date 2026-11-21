'use client';

import { useEffect, useState } from 'react';
import { ThemeMode } from '@/types';

const THEME_STORAGE_KEY = 'kv_theme_mode';

function resolveSystemTheme(): 'dark' | 'light' {
  if (typeof window === 'undefined' || !window.matchMedia) {
    return 'dark';
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function useTheme() {
  const [theme, setThemeState] = useState<ThemeMode>('system');
  const [effectiveTheme, setEffectiveTheme] = useState<'dark' | 'light'>('dark');

  useEffect(() => {
    try {
      const saved = localStorage.getItem(THEME_STORAGE_KEY) as ThemeMode | null;
      if (saved === 'light' || saved === 'dark' || saved === 'system') {
        setThemeState(saved);
      }
    } catch {
      // Ignore storage access failures in private browsing.
    }
  }, []);

  useEffect(() => {
    const apply = () => {
      const resolved: 'dark' | 'light' = theme === 'system' ? resolveSystemTheme() : theme;
      setEffectiveTheme(resolved);

      const root = document.documentElement;
      root.classList.remove('dark', 'light');
      root.classList.add(resolved);
      root.setAttribute('data-theme', resolved);
      root.style.colorScheme = resolved;
    };

    apply();

    if (theme === 'system' && typeof window !== 'undefined' && window.matchMedia) {
      const mql = window.matchMedia('(prefers-color-scheme: dark)');
      const onChange = () => apply();
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    }
  }, [theme]);

  const setTheme = (next: ThemeMode) => {
    setThemeState(next);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next);
    } catch {
      // Ignore storage write errors.
    }
  };

  const toggleTheme = () => {
    const next: ThemeMode = effectiveTheme === 'dark' ? 'light' : 'dark';
    setTheme(next);
  };

  return {
    theme,
    effectiveTheme,
    setTheme,
    toggleTheme,
  };
}
