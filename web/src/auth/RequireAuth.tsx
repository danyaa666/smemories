import { useTranslation } from "react-i18next";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { useMe } from "./useMe";

/** Layout route: signed-in users see the children, others are sent to /login and back. */
export function RequireAuth() {
  const { t } = useTranslation();
  const { pathname, search } = useLocation();
  const { data: user, isPending, isError, refetch } = useMe();
  if (isPending) return <p role="status">{t("auth.loading")}</p>;
  if (isError) {
    return (
      <>
        <p role="alert">{t("errors.network_error")}</p>
        <button type="button" onClick={() => void refetch()}>
          {t("auth.retry")}
        </button>
      </>
    );
  }
  if (!user) return <Navigate to="/login" replace state={{ from: pathname + search }} />;
  return <Outlet />;
}
