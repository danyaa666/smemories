import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, Navigate, useLocation } from "react-router-dom";
import { register } from "../api/auth";
import { GoogleButton, googleEnabled } from "../components/GoogleButton";
import { LegalConsent } from "../components/LegalConsent";
import { TERMS_VERSION } from "../legal/legal";
import { TextField } from "../components/TextField";
import { fieldError, formError, safePath } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";
import { Button } from "../components/ui/Button";

export function Register() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useMe();
  const fromState = (useLocation().state as { from?: unknown } | null)?.from;
  const from = typeof fromState === "string" ? safePath(fromState) : undefined; // e.g. /join/<token>: back there after sign-up
  const joining = from?.startsWith("/join/") === true;
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [agreed, setAgreed] = useState(false);
  const m = useMutation({
    mutationFn: () =>
      register(
        email,
        password,
        displayName,
        i18n.resolvedLanguage === "vi" ? "vi" : "en",
        TERMS_VERSION,
      ),
    onSuccess: (u) => qc.setQueryData(ME_KEY, u),
  });
  if (user) {
    // Straight after sign-up the code is on its way: land on the code screen (it starts its resend cooldown).
    return user.email_verified ? (
      <Navigate to={from ?? "/account"} replace />
    ) : (
      <Navigate to="/verify-email" replace state={{ sent: m.isSuccess, from }} />
    );
  }
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
        <LegalConsent checked={agreed} onChange={setAgreed} />
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" busy={m.isPending} disabled={!agreed}>
          {m.isPending ? t("auth.working") : t("register.submit")}
        </Button>
      </form>
      {/* Google sign-in would carry the path in a query string, and a join path holds a secret: say so instead. */}
      {joining ? (
        googleEnabled() && <p>{t("classes.join.googleHint")}</p>
      ) : (
        <GoogleButton label={t("register.google")} returnTo="/account" />
      )}
      <p>
        <Link to="/login" state={from ? { from } : undefined}>
          {t("register.haveAccount")}
        </Link>
      </p>
    </>
  );
}
