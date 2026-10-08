import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { verifyEmail } from "../api/auth";
import { errorText } from "../auth/errors";
import { ME_KEY } from "../auth/useMe";
import { useUrlToken } from "../auth/useUrlToken";

export function VerifyEmail() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const token = useUrlToken(); // first, so the token leaves the address bar before the request
  const started = useRef(false); // the token is single use: StrictMode must not send it twice
  const { mutate, isPending, isSuccess, error } = useMutation({
    mutationFn: () => verifyEmail(token),
    onSuccess: async () => {
      // The page may open before GET /v1/me has answered; a refetch would join that stale request.
      await qc.cancelQueries({ queryKey: ME_KEY });
      await qc.invalidateQueries({ queryKey: ME_KEY });
    },
  });
  useEffect(() => {
    if (token && !started.current) {
      started.current = true;
      mutate();
    }
  }, [token, mutate]);
  return (
    <>
      <h1>{t("verify.title")}</h1>
      {isSuccess ? (
        <>
          <p role="status">{t("verify.done")}</p>
          <Link to="/account">{t("verify.continue")}</Link>
        </>
      ) : !token || error ? (
        <>
          <p role="alert" className="error">
            {error ? errorText(t, error) : t("errors.invalid_token")}
          </p>
          <Link to="/account">{t("verify.continue")}</Link>
        </>
      ) : (
        isPending && <p role="status">{t("verify.working")}</p>
      )}
    </>
  );
}
