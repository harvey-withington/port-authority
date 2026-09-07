// Lightweight i18n with a reactive locale. Every user-visible string in
// the app goes through t(); the dictionaries live in ./locales/*.json.
import en from './locales/en.json'

type Translations = Record<string, string>

const STORAGE_KEY = 'pa-locale'

const locales: Record<string, Translations> = { en }

export const i18n = $state({
  locale: 'en',
})

export function t(key: string, params?: Record<string, string | number>): string {
  const dict = locales[i18n.locale] ?? locales.en
  let str = dict[key] ?? key
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      // replaceAll: a key using the same placeholder twice must not render
      // the second occurrence literally.
      str = str.replaceAll(`{${k}}`, String(v))
    }
  }
  return str
}

export function setLocale(locale: string): void {
  if (!locales[locale]) return
  i18n.locale = locale
  try {
    localStorage.setItem(STORAGE_KEY, locale)
  } catch {
    // Storage may be unavailable (private mode); the locale still applies.
  }
}

export function loadLocale(): void {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && locales[saved]) i18n.locale = saved
  } catch {
    // Storage unavailable: keep the default.
  }
}

export function registerLocale(code: string, translations: Translations): void {
  locales[code] = translations
}

export function availableLocales(): string[] {
  return Object.keys(locales)
}
