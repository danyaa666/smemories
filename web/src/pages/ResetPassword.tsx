import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { resetPassword } from "../api/auth";
import { TextField } from "../components/TextField";
import { ApiError } from "../api/client";
import { errorText, fieldError, formError } from "../auth/errors";
import { useUrlToken } from "../auth/useUrlToken";

export function ResetPassword() {
  const { t } = useTranslation();
  const token = useUrlToken();
  const [password, setPassword] = useState("");
  const m = useMutation({ mutationFn: () => resetPassword(token, password) });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate();
  };
  const badLink = !token || (m.error instanceof ApiError && m.error.code === "invalid_token");
  const error = formError(t, m.error);
  return (
    <>
      <h1>{t("reset.title")}</h1>
      {m.isSuccess ? (
        <>
          <p role="status">{t("reset.done")}</p>
          <Link to="/login">{t("nav.signIn")}</Link>
        </>
      ) : badLink ? (
        <>
          <p role="alert" className="error">
            {errorText(t, new ApiError(400, "invalid_token", ""))}
          </p>
          <Link to="/forgot-password">{t("reset.requestNew")}</Link>
        </>
      ) : (
        <form onSubmit={submit} noValidate>
          <TextField
            label={t("fields.newPassword")}
            type="password"
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
            hint={t("fields.passwordHint")}
            error={fieldError(t, m.error, "password")}
          />
          {error && (
            <p role="alert" className="error">
              {error}
            </p>
          )}
          <button type="submit" disabled={m.isPending}>
            {m.isPending ? t("auth.working") : t("reset.submit")}
          </button>
        </form>
      )}
    </>
  );
}
