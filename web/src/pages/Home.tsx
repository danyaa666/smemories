import { useTranslation } from "react-i18next";
import { ApiStatus } from "../components/ApiStatus";

export function Home() {
  const { t } = useTranslation();
  return (
    <>
      <h1>{t("app.name")}</h1>
      <p>{t("app.tagline")}</p>
      <ApiStatus />
    </>
  );
}
