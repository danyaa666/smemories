import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { createClass } from "../api/classes";
import { ApiError } from "../api/client";
import { CLASS_FIELD_OF, classErrorText } from "../classes/classErrors";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { Button } from "../components/ui/Button";
import { CLASSES_KEY } from "./Classes";

export function ClassNew() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [school, setSchool] = useState("");
  const [year, setYear] = useState("");
  const [language, setLanguage] = useState<"en" | "vi">(
    i18n.language.startsWith("vi") ? "vi" : "en",
  );
  const [yearBad, setYearBad] = useState(false);
  const m = useMutation({
    mutationFn: createClass,
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: CLASSES_KEY });
      void navigate(`/classes/${c.id}`);
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const n = year.trim() === "" ? null : Number(year);
    if (n !== null && !Number.isInteger(n)) return setYearBad(true);
    setYearBad(false);
    m.mutate({ name, school_name: school, graduation_year: n, language });
  };
  const field = m.error instanceof ApiError ? CLASS_FIELD_OF[m.error.code] : undefined;
  const msg = m.error ? classErrorText(t, m.error, "create") : undefined;
  const on = (f: string) => (field === f ? msg : undefined);
  return (
    <>
      <h1>{t("classes.newTitle")}</h1>
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
        {m.error instanceof ApiError && m.error.code === "email_not_verified" && (
          <p>
            <Link to="/verify-email">{t("classes.verifyLink")}</Link>
          </p>
        )}
        <Button type="submit" variant="primary" busy={m.isPending}>
          {m.isPending ? t("auth.working") : t("classes.form.submit")}
        </Button>
      </form>
    </>
  );
}
