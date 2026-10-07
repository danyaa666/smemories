import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";
import vi from "./locales/vi.json";

export const LANGUAGES = ["en", "vi"] as const;
export type Language = (typeof LANGUAGES)[number];

const STORAGE_KEY = "smemories.lang";

function isLanguage(value: unknown): value is Language {
  return LANGUAGES.includes(value as Language);
}

// Stored choice first, else the browser language (vi* -> vi). Storage may be blocked.
export function detectLanguage(): Language {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (isLanguage(stored)) return stored;
  } catch {
    // storage unavailable: fall through to the browser language
  }
  return navigator.language.toLowerCase().startsWith("vi") ? "vi" : "en";
}

export function setLanguage(lng: Language): Promise<unknown> {
  try {
    window.localStorage.setItem(STORAGE_KEY, lng);
  } catch {
    // the choice still applies for this session
  }
  return i18n.changeLanguage(lng);
}

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng;
});

void i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, vi: { translation: vi } },
  lng: detectLanguage(),
  fallbackLng: "en",
  interpolation: { escapeValue: false }, // React already escapes
});
document.documentElement.lang = i18n.language;

export default i18n;
