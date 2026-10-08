import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { logout } from "../api/auth";
import { errorText } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";

export function AuthNav() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { data: user } = useMe();
  const out = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      qc.clear(); // nothing cached for the old user survives
      qc.setQueryData(ME_KEY, null);
      void navigate("/login");
    },
  });
  if (user === undefined) return null; // loading or unreachable: show nothing rather than guess
  return (
    <nav aria-label={t("nav.label")} className="auth-nav">
      {user ? (
        <>
          <Link to="/account">{user.display_name}</Link>
          <button type="button" disabled={out.isPending} onClick={() => out.mutate()}>
            {t("nav.signOut")}
          </button>
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
