import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { useMe } from "../auth/useMe";

export function Account() {
  const { t } = useTranslation();
  const { data: user } = useMe();
  if (!user) return null; // RequireAuth guarantees a user
  return (
    <>
      <h1>{t("account.title")}</h1>
      <p>
        {user.display_name} · {user.email}
      </p>
      {user.email_verified ? (
        <p>{t("account.verified")}</p>
      ) : (
        <>
          <p>{t("account.notVerified")}</p>
          <Link to="/verify-email">{t("account.verifyLink")}</Link>
        </>
      )}
    </>
  );
}
