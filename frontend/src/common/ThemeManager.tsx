import { useState, useEffect, createContext, useContext, ReactNode } from 'react';

import { WindowSetDarkTheme, WindowSetLightTheme, WindowSetSystemDefaultTheme } from 'wails/runtime/runtime';

export enum ThemeType {
  SYSTEM = 'system',
  LIGHT = 'light',
  DARK = 'dark',
}

interface ThemeContextType {
  theme: ThemeType;
  effectiveTheme: ThemeType.DARK | ThemeType.LIGHT;
  setTheme: (theme: ThemeType) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const STORAGE_KEY = 'zen::theme';

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeType>(() => {
    const savedTheme = localStorage.getItem(STORAGE_KEY);
    return (savedTheme as ThemeType) || ThemeType.SYSTEM;
  });
  const [effectiveTheme, setEffectiveTheme] = useState<ThemeType.DARK | ThemeType.LIGHT>(() => {
    if (theme !== ThemeType.SYSTEM) {
      return theme;
    }
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    return prefersDark ? ThemeType.DARK : ThemeType.LIGHT;
  });

  useEffect(() => {
    if (theme !== ThemeType.SYSTEM) {
      return;
    }

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const syncSystemTheme = () => {
      setEffectiveTheme(mediaQuery.matches ? ThemeType.DARK : ThemeType.LIGHT);
    };

    if (mediaQuery.addEventListener) {
      mediaQuery.addEventListener('change', syncSystemTheme);
    } else {
      mediaQuery.addListener(syncSystemTheme);
    }

    return () => {
      if (mediaQuery.removeEventListener) {
        mediaQuery.removeEventListener('change', syncSystemTheme);
      } else {
        mediaQuery.removeListener(syncSystemTheme);
      }
    };
  }, [theme]);

  const setTheme = (newTheme: ThemeType) => {
    setThemeState(newTheme);
    localStorage.setItem(STORAGE_KEY, newTheme);
    switch (newTheme) {
      case ThemeType.LIGHT:
        WindowSetLightTheme();
        setEffectiveTheme(ThemeType.LIGHT);
        break;
      case ThemeType.DARK:
        WindowSetDarkTheme();
        setEffectiveTheme(ThemeType.DARK);
        break;
      default:
        WindowSetSystemDefaultTheme();
        setEffectiveTheme(window.matchMedia('(prefers-color-scheme: dark)').matches ? ThemeType.DARK : ThemeType.LIGHT);
    }
  };

  const value = { theme, effectiveTheme, setTheme };

  return <ThemeContext value={value}>{children}</ThemeContext>;
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}
