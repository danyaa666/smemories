import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { ClassInput } from "../api/classes";
import { ApiError } from "../api/client";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { Button } from "../components/ui/Button";
import { CLASS_FIELD_OF, classErrorText, type ClassCtx } from "./classErrors";

/** The four class fields, shared by "new class" and "edit class". Server errors land on their field; the rest above the button. */
export function ClassForm({
  initial,
  ctx,
  submitLabel,
  pending,
  error,
  onSubmit,
}: {
  initial: ClassInput;
  ctx: ClassCtx;
  submitLabel: string;
  pending: boolean;
  error: unknown;
  onSubmit: (input: ClassInput) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(initial.name);
  const [school, setSchool] = useState(initial.school_name ?? "");
  const [year, setYear] = useState(initial.graduation_year?.toString() ?? "");
  const [language, setLanguage] = useState(initial.language);
  const [yearBad, setYearBad] = useState(false);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const n = year.trim() === "" ? null : Number(year);
    if (n !== null && !Number.isInteger(n)) return setYearBad(true);
    setYearBad(false);
    onSubmit({ name, school_name: school, graduation_year: n, language });
  };
  const field = error instanceof ApiError ? CLASS_FIELD_OF[error.code] : undefined;
  const msg = error ? classErrorText(t, error, ctx) : undefined;
  const on = (f: string) => (field === f ? msg : undefined);
  return (
    <form onSubmit={submit} noValidate>
      <TextField
        label={t("classes.form.name")}
        value={name}
        onChange={setName}
        autoComplete="off"
        error={on("name")}
      />
      <TextField
        label={t("classes.form.school")}
        value={school}
        onChange={setSchool}
        autoComplete="organization"
        error={on("school")}
      />
      <TextField
        label={t("classes.form.year")}
        type="number"
        value={year}
        onChange={setYear}
        autoComplete="off"
        error={yearBad ? t("errors.invalid_graduation_year") : on("year")}
      />
      <SelectField
        label={t("classes.form.language")}
        value={language}
        onChange={(v) => setLanguage(v === "vi" ? "vi" : "en")}
        options={[
          { value: "en", label: t("language.en") },
          { value: "vi", label: t("language.vi") },
        ]}
      />
      {msg && !field && (
        <p role="alert" className="error">
          {msg}
        </p>
      )}
      {error instanceof ApiError && error.code === "email_not_verified" && (
        <p>
          <Link to="/verify-email">{t("classes.verifyLink")}</Link>
        </p>
      )}
      <Button type="submit" variant="primary" busy={pending}>
        {pending ? t("auth.working") : submitLabel}
      </Button>
    </form>
  );
}
