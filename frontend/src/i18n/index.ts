import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

import { GetLocale, SetLocale } from 'wails/go/config/Config';

import arSA from './locales/ar-SA.json';
import bgBG from './locales/bg-BG.json';
import caES from './locales/ca-ES.json';
import csCZ from './locales/cs-CZ.json';
import daDK from './locales/da-DK.json';
import deDE from './locales/de-DE.json';
import elGR from './locales/el-GR.json';
import enUS from './locales/en-US.json';
import esES from './locales/es-ES.json';
import euES from './locales/eu-ES.json';
import fiFI from './locales/fi-FI.json';
import frFR from './locales/fr-FR.json';
import hrHR from './locales/hr-HR.json';
import huHU from './locales/hu-HU.json';
import idID from './locales/id-ID.json';
import itIT from './locales/it-IT.json';
import jaJP from './locales/ja-JP.json';
import kkKZ from './locales/kk-KZ.json';
import koKR from './locales/ko-KR.json';
import nbNO from './locales/nb-NO.json';
import nlNL from './locales/nl-NL.json';
import ptPT from './locales/pt-PT.json';
import roRO from './locales/ro-RO.json';
import ruRU from './locales/ru-RU.json';
import skSK from './locales/sk-SK.json';
import trTR from './locales/tr-TR.json';
import ukUA from './locales/uk-UA.json';
import viVN from './locales/vi-VN.json';
import zhCN from './locales/zh-CN.json';
import zhTW from './locales/zh-TW.json';

export const SUPPORTED_LOCALES = [
  'ar',
  'ar-SA',
  'bg',
  'bg-BG',
  'ca',
  'ca-ES',
  'cs',
  'cs-CZ',
  'da',
  'da-DK',
  'en',
  'en-US',
  'es',
  'es-ES',
  'eu',
  'eu-ES',
  'fi',
  'fi-FI',
  'de',
  'de-DE',
  'el',
  'el-GR',
  'hr',
  'hr-HR',
  'hu',
  'hu-HU',
  'id',
  'id-ID',
  'it',
  'it-IT',
  'ko',
  'ko-KR',
  'nl',
  'nl-NL',
  'nb',
  'nb-NO',
  'pt',
  'pt-PT',
  'ro',
  'ro-RO',
  'tr',
  'tr-TR',
  'kk',
  'kk-KZ',
  'sk',
  'sk-SK',
  'uk',
  'uk-UA',
  'ru',
  'ru-RU',
  'zh',
  'zh-CN',
  'zh-TW',
  'ja',
  'ja-JP',
  'fr',
  'fr-FR',
  'vi',
  'vi-VN',
] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
export const FALLBACK_LOCALE: SupportedLocale = 'en-US';

export interface LocaleItem {
  value: SupportedLocale;
  label: string;
}

export const LOCALE_LABELS: LocaleItem[] = [
  { value: 'ar-SA', label: 'العربية' },
  { value: 'bg-BG', label: 'Български' },
  { value: 'ca-ES', label: 'Català' },
  { value: 'cs-CZ', label: 'Čeština' },
  { value: 'da-DK', label: 'Dansk' },
  { value: 'en-US', label: 'English' },
  { value: 'de-DE', label: 'Deutsch' },
  { value: 'el-GR', label: 'Ελληνικά' },
  { value: 'es-ES', label: 'Español' },
  { value: 'eu-ES', label: 'Euskara' },
  { value: 'fi-FI', label: 'Suomi' },
  { value: 'fr-FR', label: 'Français' },
  { value: 'hr-HR', label: 'Hrvatski' },
  { value: 'hu-HU', label: 'Magyar' },
  { value: 'id-ID', label: 'Bahasa Indonesia' },
  { value: 'it-IT', label: 'Italiano' },
  { value: 'kk-KZ', label: 'Қазақша' },
  { value: 'ko-KR', label: '한국어' },
  { value: 'nl-NL', label: 'Nederlands' },
  { value: 'nb-NO', label: 'Norsk (bokmål)' },
  { value: 'pt-PT', label: 'Português' },
  { value: 'ro-RO', label: 'Română' },
  { value: 'ru-RU', label: 'Русский' },
  { value: 'sk-SK', label: 'Slovenčina' },
  { value: 'tr-TR', label: 'Türkçe' },
  { value: 'uk-UA', label: 'Українська' },
  { value: 'vi-VN', label: 'Tiếng Việt' },
  { value: 'zh-CN', label: '中文（简体）' },
  { value: 'zh-TW', label: '中文（繁體）' },
  { value: 'ja-JP', label: '日本語' },
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
      ar: { translation: arSA },
      'ar-SA': { translation: arSA },
      bg: { translation: bgBG },
      'bg-BG': { translation: bgBG },
      ca: { translation: caES },
      'ca-ES': { translation: caES },
      cs: { translation: csCZ },
      'cs-CZ': { translation: csCZ },
      da: { translation: daDK },
      'da-DK': { translation: daDK },
      en: { translation: enUS },
      'en-US': { translation: enUS },
      es: { translation: esES },
      'es-ES': { translation: esES },
      eu: { translation: euES },
      'eu-ES': { translation: euES },
      fi: { translation: fiFI },
      'fi-FI': { translation: fiFI },
      de: { translation: deDE },
      'de-DE': { translation: deDE },
      el: { translation: elGR },
      'el-GR': { translation: elGR },
      hr: { translation: hrHR },
      'hr-HR': { translation: hrHR },
      hu: { translation: huHU },
      'hu-HU': { translation: huHU },
      id: { translation: idID },
      'id-ID': { translation: idID },
      it: { translation: itIT },
      'it-IT': { translation: itIT },
      fr: { translation: frFR },
      'fr-FR': { translation: frFR },
      kk: { translation: kkKZ },
      'kk-KZ': { translation: kkKZ },
      ko: { translation: koKR },
      'ko-KR': { translation: koKR },
      nl: { translation: nlNL },
      'nl-NL': { translation: nlNL },
      nb: { translation: nbNO },
      'nb-NO': { translation: nbNO },
      pt: { translation: ptPT },
      'pt-PT': { translation: ptPT },
      ro: { translation: roRO },
      'ro-RO': { translation: roRO },
      ru: { translation: ruRU },
      'ru-RU': { translation: ruRU },
      sk: { translation: skSK },
      'sk-SK': { translation: skSK },
      tr: { translation: trTR },
      'tr-TR': { translation: trTR },
      uk: { translation: ukUA },
      'uk-UA': { translation: ukUA },
      vi: { translation: viVN },
      'vi-VN': { translation: viVN },
      zh: { translation: zhCN },
      'zh-CN': { translation: zhCN },
      'zh-TW': { translation: zhTW },
      ja: { translation: jaJP },
      'ja-JP': { translation: jaJP },
    },
    lng: locale,
    fallbackLng: FALLBACK_LOCALE,
    missingKeyHandler: (lngs, ns, key) => {
      // Fallback to English happens automatically; log it so incomplete
      // translations are easy to spot during development.
      console.warn(`i18n: missing translation key "${key}" in ${lngs.join(', ')} (namespace "${ns}")`);
    },
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
