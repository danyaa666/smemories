import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { forgotPassword } from "../api/auth";
import { TextField } from "../components/TextField";
import { fieldError, formError } from "../auth/errors";

export function ForgotPassword() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const m = useMutation({
    mutationFn: () => forgotPassword(email),
    // The API answers 202 whether or not the account exists, so every well-formed address goes on.
    onSuccess: () => navigate("/reset-password", { state: { email, sent: true } }),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate();
  };
  const error = formError(t, m.error);
  return (
    <>
      <h1>{t("forgot.title")}</h1>
      <p>{t("forgot.intro")}</p>
      <form onSubmit={submit} noValidate>
        <TextField
          label={t("fields.email")}
          type="email"
          value={email}
          onChange={setEmail}
          autoComplete="email"
          error={fieldError(t, m.error, "email")}
        />
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <button type="submit" disabled={m.isPending}>
          {m.isPending ? t("auth.working") : t("forgot.submit")}
        </button>
      </form>
      <p>
        <Link to="/login">{t("forgot.back")}</Link>
      </p>
    </>
  );
}
