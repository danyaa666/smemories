import { useTranslation } from "react-i18next";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { useMe } from "./useMe";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";

/** Layout route: signed-in users see the children, others are sent to /login and back. */
export function RequireAuth() {
  const { t } = useTranslation();
  const { pathname, search } = useLocation();
  const { data: user, isPending, isError, refetch } = useMe();
  // Neither state has the signed-in shell yet (it needs the user), so each brings its own landmark.
  if (isPending) {
    return (
      <main className="plain-main">
        <p role="status">{t("auth.loading")}</p>
      </main>
    );
  }
  if (isError) {
    return (
      <main className="plain-main">
        <Banner
          variant="error"
          action={<Button onClick={() => void refetch()}>{t("auth.retry")}</Button>}
        >
          {t("errors.network_error")}
        </Banner>
      </main>
    );
  }
  if (!user) return <Navigate to="/login" replace state={{ from: pathname + search }} />;
  return <Outlet />;
}
