import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, Navigate } from "react-router-dom";
import { register } from "../api/auth";
import { GoogleButton } from "../components/GoogleButton";
import { TextField } from "../components/TextField";
import { fieldError, formError } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";

export function Register() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useMe();
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const m = useMutation({
    mutationFn: () => register(email, password, displayName),
    onSuccess: (u) => qc.setQueryData(ME_KEY, u),
  });
  if (user) return <Navigate to="/account" replace />;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate();
  };
  const error = formError(t, m.error);
  return (
    <>
      <h1>{t("register.title")}</h1>
      <form onSubmit={submit} noValidate>
        <TextField
          label={t("fields.displayName")}
          value={displayName}
          onChange={setDisplayName}
          autoComplete="name"
          error={fieldError(t, m.error, "display_name")}
        />
        <TextField
          label={t("fields.email")}
          type="email"
          value={email}
          onChange={setEmail}
          autoComplete="email"
          error={fieldError(t, m.error, "email")}
        />
        <TextField
          label={t("fields.password")}
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
          {m.isPending ? t("auth.working") : t("register.submit")}
        </button>
      </form>
      <GoogleButton label={t("register.google")} returnTo="/account" />
      <p>
        <Link to="/login">{t("register.haveAccount")}</Link>
      </p>
    </>
  );
}
