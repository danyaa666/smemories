import { useTranslation } from "react-i18next";
import { Route, Routes } from "react-router-dom";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { Home } from "./pages/Home";
import { NotFound } from "./pages/NotFound";

export function App() {
  const { t } = useTranslation();
  return (
    <>
      <header className="app-header">
        <strong>{t("app.name")}</strong>
        <LanguageSwitcher />
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
    </>
  );
}
