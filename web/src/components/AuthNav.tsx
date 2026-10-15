import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { errorText } from "../auth/errors";
import { useMe } from "../auth/useMe";
import { Button } from "./ui/Button";
import { useSignOut } from "./useSignOut";

/** The header of the public pages: sign in and register, or the account links when signed in. */
export function AuthNav() {
  const { t } = useTranslation();
  const { data: user } = useMe();
  const out = useSignOut();
  if (user === undefined) return null; // loading or unreachable: show nothing rather than guess
  return (
    <nav aria-label={t("nav.label")} className="auth-nav">
      {user ? (
        <>
          <Link to="/yearbooks">{t("nav.yearbooks")}</Link>
          <Link to="/account">{user.display_name}</Link>
          <Button variant="ghost" disabled={out.isPending} onClick={() => out.mutate()}>
            {t("nav.signOut")}
          </Button>
          {out.isError && <span role="alert">{errorText(t, out.error)}</span>}
        </>
      ) : (
        <>
          <Link to="/login">{t("nav.signIn")}</Link>
          <Link to="/register">{t("nav.register")}</Link>
        </>
      )}
    </nav>
  );
}
