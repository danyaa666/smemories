import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import type { YearbookCreate } from "../api/yearbooks";
import { fieldError, formError } from "../auth/errors";
import { SelectField } from "./SelectField";
import { TextField } from "./TextField";

export const PAGE_SIZES = ["A5", "A4", "Letter"] as const;

type Props = {
  initial: YearbookCreate & { school_name: string; class_name: string; motto: string };
  submitLabel: string;
  pending: boolean;
  error: unknown;
  onSubmit: (body: YearbookCreate) => void;
};

/** Book details: used to create a book and to edit it (the PATCH takes the same body). */
export function BookForm({ initial, submitLabel, pending, error, onSubmit }: Props) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(initial.title);
  const [school, setSchool] = useState(initial.school_name);
  const [klass, setKlass] = useState(initial.class_name);
  const [year, setYear] = useState(initial.graduation_year?.toString() ?? "");
  const [motto, setMotto] = useState(initial.motto);
  const [language, setLanguage] = useState(initial.language);
  const [pageSize, setPageSize] = useState(initial.page_size ?? "A5");
  const [yearBad, setYearBad] = useState(false);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const n = year.trim() === "" ? null : Number(year);
    if (n !== null && !Number.isInteger(n)) return setYearBad(true);
    setYearBad(false);
    onSubmit({
      title,
      school_name: school,
      class_name: klass,
      graduation_year: n,
      motto,
      language,
      page_size: pageSize,
    });
  };
  const err = (f: string) => fieldError(t, error, f);
  return (
    <form onSubmit={submit} noValidate>
      <TextField
        label={t("book.title")}
        value={title}
        onChange={setTitle}
        autoComplete="off"
        error={err("title")}
      />
      <TextField
        label={t("book.school")}
        value={school}
        onChange={setSchool}
        autoComplete="organization"
        error={err("school_name")}
      />
      <TextField
        label={t("book.class")}
        value={klass}
        onChange={setKlass}
        autoComplete="off"
        error={err("class_name")}
      />
      <TextField
        label={t("book.year")}
        type="number"
        value={year}
        onChange={setYear}
        autoComplete="off"
        error={yearBad ? t("errors.invalid_graduation_year") : err("graduation_year")}
      />
      <TextField
        label={t("book.motto")}
        value={motto}
        onChange={setMotto}
        autoComplete="off"
        error={err("motto")}
      />
      <SelectField
        label={t("book.language")}
        value={language}
        onChange={(v) => setLanguage(v === "vi" ? "vi" : "en")}
        options={[
          { value: "en", label: t("language.en") },
          { value: "vi", label: t("language.vi") },
        ]}
      />
      <SelectField
        label={t("book.pageSize")}
        value={pageSize}
        onChange={(v) => setPageSize(PAGE_SIZES.find((s) => s === v) ?? "A5")}
        options={PAGE_SIZES.map((s) => ({ value: s, label: t(`book.pageSizes.${s}`) }))}
      />
      {formError(t, error) && (
        <p role="alert" className="error">
          {formError(t, error)}
        </p>
      )}
      <button type="submit" disabled={pending}>
        {pending ? t("auth.working") : submitLabel}
      </button>
    </form>
  );
}
