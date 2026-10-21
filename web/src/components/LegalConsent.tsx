import { Trans, useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

// New tab and no opener: the form the user is filling in (or the token page of a friend) stays where it is.
const page = (to: string) => <Link to={to} target="_blank" rel="noopener" />;
const pages = { terms: page("/terms"), privacy: page("/privacy") };

/** The required sign-up / accept checkbox: "I agree to the Terms of Use and the Privacy Policy", both titles link to the pages. */
export function LegalConsent({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="legal-consent">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span>
        <Trans i18nKey="legal.agree" components={pages} />
      </span>
    </label>
  );
}

/** The one line under the send button of the public note form. */
export function LegalNotice() {
  return (
    <p className="note-muted">
      <Trans i18nKey="legal.notice" components={pages} />
    </p>
  );
}

/** Terms and Privacy links for the footer of the public pages and the signed-in shell. */
export function LegalLinks() {
  const { t } = useTranslation();
  return (
    <div className="legal-links">
      <Link to="/terms">{t("legal.terms")}</Link>
      <Link to="/privacy">{t("legal.privacy")}</Link>
    </div>
  );
}
