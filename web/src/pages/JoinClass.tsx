import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, Navigate, useParams } from "react-router-dom";
import { acceptInvite, lookupInvite } from "../api/classes";
import { ApiError } from "../api/client";
import { errorText } from "../auth/errors";
import { useMe } from "../auth/useMe";
import { classErrorText } from "../classes/classErrors";
import { CreatePageButton } from "../classes/CreatePageButton";
import { Button, buttonClass } from "../components/ui/Button";
import { CLASSES_KEY } from "./Classes";

/**
 * /join/:token. The token is a bearer secret: it lives in this path and in memory only. It goes to the API in POST bodies,
 * is never put in a query string or storage, and every 404 gets the one neutral message (unknown, expired, revoked alike).
 */
export function JoinClass() {
  const { t } = useTranslation();
  const { token = "" } = useParams();
  const qc = useQueryClient();
  const { data: user, isPending: mePending } = useMe();
  const from = `/join/${token}`; // router state: in memory, back here after sign-up or sign-in
  const preview = useQuery({
    queryKey: ["invite-preview", token],
    queryFn: () => lookupInvite(token),
    retry: false,
    staleTime: Infinity,
  });
  const accept = useMutation({
    mutationFn: () => acceptInvite(token),
    onSuccess: () => void qc.invalidateQueries({ queryKey: CLASSES_KEY }),
  });

  if (preview.isPending) return <p role="status">{t("auth.loading")}</p>;
  if (preview.isError) {
    const dead = preview.error instanceof ApiError && preview.error.status === 404;
    return (
      <>
        <h1>{t(dead ? "classes.join.invalidTitle" : "classes.join.title")}</h1>
        <p role="alert" className="error">
          {dead ? t("classes.join.invalid") : errorText(t, preview.error)}
        </p>
        {dead ? (
          <p>{t("classes.join.invalidHelp")}</p>
        ) : (
          <Button onClick={() => void preview.refetch()}>{t("auth.retry")}</Button>
        )}
      </>
    );
  }
  const { name, school_name: school } = preview.data;

  if (accept.isSuccess) {
    return (
      <>
        <h1>{t("classes.join.joined", { name: accept.data.name })}</h1>
        <p role="status">{t("classes.join.joinedHelp")}</p>
        <CreatePageButton cls={accept.data} />
        <p>
          <Link to={`/classes/${accept.data.id}`}>{t("classes.join.toClass")}</Link>
        </p>
      </>
    );
  }
  const head = (
    <>
      <h1>{t("classes.join.heading", { name })}</h1>
      {school && <p>{school}</p>}
    </>
  );
  if (mePending) return <>{head}</>;
  if (!user) {
    return (
      <>
        {head}
        <p>{t("classes.join.signInFirst")}</p>
        <p className="row-actions">
          <Link to="/register" state={{ from }} className={buttonClass("primary")}>
            {t("nav.register")}
          </Link>
          <Link to="/login" state={{ from }} className={buttonClass("secondary")}>
            {t("nav.signIn")}
          </Link>
        </p>
      </>
    );
  }
  if (user.terms?.required) return <Navigate to="/accept-terms" replace state={{ from }} />;
  if (!user.email_verified) {
    return (
      <>
        {head}
        <p role="alert" className="error">
          {t("errors.email_not_verified")}
        </p>
        <Link to="/verify-email" state={{ from }} className={buttonClass("primary")}>
          {t("classes.verifyLink")}
        </Link>
      </>
    );
  }
  return (
    <>
      {head}
      {accept.error && (
        <p role="alert" className="error">
          {classErrorText(t, accept.error, "join")}
        </p>
      )}
      <Button variant="primary" busy={accept.isPending} onClick={() => accept.mutate()}>
        {t("classes.join.accept")}
      </Button>
    </>
  );
}
