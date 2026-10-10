import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { resendVerification, verifyEmail } from "../api/auth";
import { dropTokenParam } from "../auth/dropTokenParam";
import { errorText, fieldError, formError } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";
import { CODE_LENGTH, CodeField } from "../components/CodeField";
import { ResendCode } from "../components/ResendCode";

export function VerifyEmail() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useMe();
  const justSent = (useLocation().state as { sent?: boolean } | null)?.sent === true;
  const [code, setCode] = useState("");
  const [short, setShort] = useState(false); // fewer than 6 digits: say so without spending an attempt
  useEffect(dropTokenParam, []);
  const refreshMe = async () => {
    // The page may open before GET /v1/me has answered; a refetch would join that stale request.
    await qc.cancelQueries({ queryKey: ME_KEY });
    await qc.invalidateQueries({ queryKey: ME_KEY });
  };
  const verify = useMutation({ mutationFn: verifyEmail, onSuccess: refreshMe });
  const resend = useMutation({
    mutationFn: resendVerification,
    onSuccess: (r) => {
      setCode("");
      verify.reset();
      if (r?.already_verified) void refreshMe();
    },
  });
  const submit = (c: string) => {
    if (verify.isPending) return;
    if (c.length < CODE_LENGTH) return setShort(true);
    resend.reset();
    verify.mutate(c);
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit(code);
  };

  if (verify.isSuccess || user?.email_verified) {
    return (
      <>
        <h1>{t("verify.title")}</h1>
        <p role="status">{t("verify.done")}</p>
        <Link to="/account">{t("verify.continue")}</Link>
      </>
    );
  }
  const error = formError(t, verify.error);
  return (
    <>
      <h1>{t("verify.title")}</h1>
      <p>{t("verify.sentTo", { email: user?.email ?? "" })}</p>
      <form onSubmit={onSubmit} noValidate>
        <CodeField
          label={t("code.label")}
          hint={t("code.hint")}
          value={code}
          onChange={(v) => {
            setCode(v);
            setShort(false);
          }}
          onComplete={submit}
          disabled={verify.isPending}
          focusOnMount
          error={short ? t("errors.code_short") : fieldError(t, verify.error, "code")}
        />
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <button type="submit" disabled={verify.isPending}>
          {verify.isPending ? t("auth.working") : t("verify.submit")}
        </button>
      </form>
      <p>
        <ResendCode
          storageKey="resendUntil:verify"
          startCooling={justSent}
          pending={resend.isPending}
          onSend={() =>
            resend.mutateAsync().then(
              (r) => !r?.already_verified,
              () => false,
            )
          }
        />
      </p>
      <p>
        <small>{t("verify.resendLimit")}</small>
      </p>
      {resend.isSuccess && !resend.data?.already_verified && (
        <p role="status">{t("code.resent")}</p>
      )}
      {resend.isError && (
        <p role="alert" className="error">
          {errorText(t, resend.error)}
        </p>
      )}
    </>
  );
}
