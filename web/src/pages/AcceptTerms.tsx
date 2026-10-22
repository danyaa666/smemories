import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useLocation } from "react-router-dom";
import { acceptTerms } from "../api/auth";
import { errorText, safePath } from "../auth/errors";
import { ME_KEY, useMe } from "../auth/useMe";
import { LegalConsent } from "../components/LegalConsent";
import { Button } from "../components/ui/Button";
import { useSignOut } from "../components/useSignOut";
import { TERMS_VERSION } from "../legal/legal";

/** /accept-terms: RequireAuth sends a signed-in user here while get-me says terms.required; accepting returns to where they were going. */
export function AcceptTerms() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useMe();
  const from = safePath((useLocation().state as { from?: unknown } | null)?.from);
  const [agreed, setAgreed] = useState(false);
  const m = useMutation({
    mutationFn: () => acceptTerms(TERMS_VERSION),
    onSuccess: (me) => qc.setQueryData(ME_KEY, me),
  });
  const out = useSignOut();
  if (user && !user.terms?.required) return <Navigate to={from} replace />;
  return (
    <>
      <h1>{t(user?.terms?.accepted_version ? "accept.titleChanged" : "accept.titleFirst")}</h1>
      <p>{t("accept.body")}</p>
      <LegalConsent checked={agreed} onChange={setAgreed} />
      {(m.error || out.error) && (
        <p role="alert" className="error">
          {errorText(t, m.error ?? out.error)}
        </p>
      )}
      <div className="legal-actions">
        <Button variant="primary" disabled={!agreed} busy={m.isPending} onClick={() => m.mutate()}>
          {t("accept.submit")}
        </Button>
        <Button busy={out.isPending} onClick={() => out.mutate()}>
          {t("nav.signOut")}
        </Button>
      </div>
    </>
  );
}
