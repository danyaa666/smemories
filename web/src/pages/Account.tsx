import { useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { resendVerification } from "../api/auth";
import { errorText } from "../auth/errors";
import { useMe } from "../auth/useMe";

export function Account() {
  const { t } = useTranslation();
  const { data: user } = useMe();
  const resend = useMutation({ mutationFn: resendVerification });
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
          <button type="button" disabled={resend.isPending} onClick={() => resend.mutate()}>
            {t("account.resend")}
          </button>
          <p>
            <small>{t("account.resendLimit")}</small>
          </p>
          {resend.isSuccess && (
            <p role="status">
              {resend.data?.already_verified ? t("account.resendAlready") : t("account.resendSent")}
            </p>
          )}
          {resend.isError && (
            <p role="alert" className="error">
              {errorText(t, resend.error)}
            </p>
          )}
        </>
      )}
    </>
  );
}
