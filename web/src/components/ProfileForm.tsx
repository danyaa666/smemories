import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { replaceProfile, type Yearbook } from "../api/yearbooks";
import { fieldError, formError } from "../auth/errors";
import { TextField } from "./TextField";

/** The owner's profile page. PUT replaces everything, so the photo is sent back on every save. */
export function ProfileForm({
  yearbook,
  onSaved,
}: {
  yearbook: Yearbook;
  onSaved: (y: Yearbook) => void;
}) {
  const { t } = useTranslation();
  const p = yearbook.profile;
  const [fullName, setFullName] = useState(p.full_name);
  const [nickname, setNickname] = useState(p.nickname);
  const [birthday, setBirthday] = useState(p.birthday ?? "");
  const [quote, setQuote] = useState(p.quote);
  const [hobbies, setHobbies] = useState(p.hobbies);
  const [plans, setPlans] = useState(p.future_plans);
  const m = useMutation({
    mutationFn: () =>
      replaceProfile(yearbook.id, {
        full_name: fullName,
        nickname,
        birthday: birthday || null,
        quote,
        hobbies,
        future_plans: plans,
        // the latest server state, not the value the form opened with: the photo may have changed since
        photo_media_id: yearbook.profile.photo_media_id,
      }),
    onSuccess: onSaved,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate();
  };
  const err = (f: string) => fieldError(t, m.error, f);
  return (
    <form onSubmit={submit} noValidate>
      <TextField
        label={t("profile.fullName")}
        value={fullName}
        onChange={setFullName}
        autoComplete="name"
        error={err("full_name")}
      />
      <TextField
        label={t("profile.nickname")}
        value={nickname}
        onChange={setNickname}
        autoComplete="nickname"
        error={err("nickname")}
      />
      <TextField
        label={t("profile.birthday")}
        type="date"
        value={birthday}
        onChange={setBirthday}
        autoComplete="bday"
        error={err("birthday")}
      />
      <TextField
        label={t("profile.quote")}
        multiline
        value={quote}
        onChange={setQuote}
        autoComplete="off"
        error={err("quote")}
      />
      <TextField
        label={t("profile.hobbies")}
        multiline
        value={hobbies}
        onChange={setHobbies}
        autoComplete="off"
        error={err("hobbies")}
      />
      <TextField
        label={t("profile.plans")}
        multiline
        value={plans}
        onChange={setPlans}
        autoComplete="off"
        error={err("future_plans")}
      />
      {formError(t, m.error) && (
        <p role="alert" className="error">
          {formError(t, m.error)}
        </p>
      )}
      <button type="submit" disabled={m.isPending}>
        {m.isPending ? t("auth.working") : t("profile.save")}
      </button>
      {m.isSuccess && <p role="status">{t("book.saved")}</p>}
    </form>
  );
}
