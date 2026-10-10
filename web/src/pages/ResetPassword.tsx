import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { forgotPassword, resetPassword } from "../api/auth";
import { ApiError } from "../api/client";
import { errorText, fieldError, formError } from "../auth/errors";
import { CODE_LENGTH, CodeField } from "../components/CodeField";
import { ResendCode } from "../components/ResendCode";
import { TextField } from "../components/TextField";

export function ResetPassword() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const state = useLocation().state as { email?: string; sent?: boolean } | null;
  const [email, setEmail] = useState(state?.email ?? "");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [short, setShort] = useState(false);
  const [sent, setSent] = useState(state?.sent === true);
  const m = useMutation({
    mutationFn: () => resetPassword(email, code, password),
    onSuccess: () => navigate("/login", { replace: true, state: { reset: true } }),
  });
  const resend = useMutation({ mutationFn: () => forgotPassword(email) });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (code.length < CODE_LENGTH) return setShort(true);
    m.mutate();
  };
  // busy: the code was already used up, so the generic "try again" would send the student in circles
  const error =
    m.error instanceof ApiError && m.error.code === "busy"
      ? t("reset.busy")
      : formError(t, m.error);
  const emailError = fieldError(t, m.error, "email") ?? fieldError(t, resend.error, "email");
  return (
    <>
      <h1>{t("reset.title")}</h1>
      {sent && <p role="status">{t("reset.sent")}</p>}
      <form onSubmit={submit} noValidate>
        <TextField
          label={t("fields.email")}
          type="email"
          value={email}
          onChange={setEmail}
          autoComplete="email"
          error={emailError}
        />
        <CodeField
          label={t("code.label")}
          hint={t("code.hint")}
          value={code}
          onChange={(v) => {
            setCode(v);
            setShort(false);
          }}
          focusOnMount={Boolean(state?.email)}
          error={short ? t("errors.invalid_code") : fieldError(t, m.error, "code")}
        />
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
      <p>
        <ResendCode
          startCooling={state?.sent === true}
          pending={resend.isPending}
          onSend={() =>
            resend.mutateAsync().then(
              () => {
                setSent(true);
                setCode("");
                m.reset();
                return true;
              },
              () => false,
            )
          }
        />
      </p>
      {resend.isError && !emailError && (
        <p role="alert" className="error">
          {errorText(t, resend.error)}
        </p>
      )}
      <p>
        <Link to="/login">{t("forgot.back")}</Link>
      </p>
    </>
  );
}
