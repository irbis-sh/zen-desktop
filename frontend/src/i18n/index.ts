import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

import { GetLocale, SetLocale } from 'wails/go/config/Config';

import deDE from './locales/de-DE.json';
import enUS from './locales/en-US.json';
import frFR from './locales/fr-FR.json';
import itIT from './locales/it-IT.json';
import jaJP from './locales/ja-JP.json';
import kkKZ from './locales/kk-KZ.json';
import ruRU from './locales/ru-RU.json';
import trTR from './locales/tr-TR.json';
import zhCN from './locales/zh-CN.json';
import zhTW from './locales/zh-TW.json';

export const SUPPORTED_LOCALES = [
  'en',
  'en-US',
  'de',
  'de-DE',
  'it',
  'it-IT',
  'tr',
  'tr-TR',
  'kk',
  'kk-KZ',
  'ru',
  'ru-RU',
  'zh',
  'zh-CN',
  'zh-TW',
  'ja',
  'ja-JP',
  'fr',
  'fr-FR',
] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
export const FALLBACK_LOCALE: SupportedLocale = 'en-US';

export interface LocaleItem {
  value: SupportedLocale;
  label: string;
}

export const LOCALE_LABELS: LocaleItem[] = [
  { value: 'en-US', label: 'English' },
  { value: 'de-DE', label: 'Deutsch' },
  { value: 'kk-KZ', label: 'Қазақша' },
  { value: 'ru-RU', label: 'Русский' },
  { value: 'zh-CN', label: '中文（简体）' },
  { value: 'zh-TW', label: '中文（繁體）' },
  { value: 'ja-JP', label: '日本語' },
  { value: 'it-IT', label: 'Italiano' },
  { value: 'tr-TR', label: 'Türkçe' },
  { value: 'fr-FR', label: 'Français' },
];

// Sort language options into a consistent, user-friendly alphabetical order.
// Uses Unicode root collation ("und") to keep the order stable and identical for everyone.
const localeLabelsCollator = new Intl.Collator('und', {
  usage: 'sort',
  sensitivity: 'base',
});
LOCALE_LABELS.sort((a, b) => localeLabelsCollator.compare(a.label, b.label));

export function detectSystemLocale(): SupportedLocale {
  const browserLang = navigator.language;
  const detected = (SUPPORTED_LOCALES as readonly string[]).includes(browserLang)
    ? (browserLang as SupportedLocale)
    : FALLBACK_LOCALE;

  return detected;
}

export function getCurrentLocale(): SupportedLocale {
  return (i18n.language as SupportedLocale) || FALLBACK_LOCALE;
}

export async function changeLocale(locale: SupportedLocale) {
  const normalized = SUPPORTED_LOCALES.includes(locale) ? locale : FALLBACK_LOCALE;
  await i18n.changeLanguage(normalized);
  await SetLocale(normalized);
}

export async function initI18n() {
  let locale = await GetLocale();
  if (locale === '') {
    const detected = detectSystemLocale();
    await SetLocale(detected);
    locale = detected;
  }

  return i18n.use(initReactI18next).init({
    resources: {
      en: { translation: enUS },
      'en-US': { translation: enUS },
      de: { translation: deDE },
      'de-DE': { translation: deDE },
      kk: { translation: kkKZ },
      'kk-KZ': { translation: kkKZ },
      ru: { translation: ruRU },
      'ru-RU': { translation: ruRU },
      zh: { translation: zhCN },
      'zh-CN': { translation: zhCN },
      'zh-TW': { translation: zhTW },
      ja: { translation: jaJP },
      'ja-JP': { translation: jaJP },
      it: { translation: itIT },
      'it-IT': { translation: itIT },
      tr: { translation: trTR },
      'tr-TR': { translation: trTR },
      fr: { translation: frFR },
      'fr-FR': { translation: frFR },
    },
    lng: locale,
    fallbackLng: FALLBACK_LOCALE,
    returnNull: false,
    returnEmptyString: false,
    interpolation: {
      escapeValue: false,
    },
    react: {
      useSuspense: false,
    },
  });
}
