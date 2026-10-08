import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, Navigate, useLocation, useSearchParams } from "react-router-dom";
import { login } from "../api/auth";
import { GoogleButton } from "../components/GoogleButton";
import { TextField } from "../components/TextField";
import { errorText, safePath } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";

const OIDC_ERRORS = ["oidc_state", "oidc_denied", "oidc_failed", "email_unverified"];

export function Login() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const from = safePath((useLocation().state as { from?: unknown } | null)?.from);
  const oidc = useSearchParams()[0].get("error");
  const { data: user } = useMe();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const m = useMutation({
    mutationFn: () => login(email, password),
    onSuccess: (u) => qc.setQueryData(ME_KEY, u),
  });
  if (user) return <Navigate to={from} replace />;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate();
  };
  const error = m.error ? errorText(t, m.error) : undefined;
  return (
    <>
      <h1>{t("login.title")}</h1>
      {oidc && (
        <p role="alert" className="error">
          {t(OIDC_ERRORS.includes(oidc) ? `login.oidc.${oidc}` : "login.oidc.unknown")}
        </p>
      )}
      <form onSubmit={submit} noValidate>
        <TextField
          label={t("fields.email")}
          type="email"
          value={email}
          onChange={setEmail}
          autoComplete="email"
        />
        <TextField
          label={t("fields.password")}
          type="password"
          value={password}
          onChange={setPassword}
          autoComplete="current-password"
        />
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <button type="submit" disabled={m.isPending}>
          {m.isPending ? t("auth.working") : t("login.submit")}
        </button>
      </form>
      <GoogleButton label={t("login.google")} returnTo={from} />
      <p>
        <Link to="/forgot-password">{t("login.forgot")}</Link>
      </p>
      <p>
        <Link to="/register">{t("login.noAccount")}</Link>
      </p>
    </>
  );
}
