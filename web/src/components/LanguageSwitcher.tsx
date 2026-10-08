import { useTranslation } from "react-i18next";
import { LANGUAGES, setLanguage } from "../i18n";

export function LanguageSwitcher() {
  const { t, i18n } = useTranslation();
  return (
    <div role="group" aria-label={t("language.label")} className="lang-switcher">
      {LANGUAGES.map((lng) => (
        <button
          key={lng}
          type="button"
          lang={lng}
          aria-label={t(`language.${lng}`)}
          aria-pressed={i18n.language === lng}
          onClick={() => void setLanguage(lng)}
        >
          {lng.toUpperCase()}
        </button>
      ))}
    </div>
  );
}
