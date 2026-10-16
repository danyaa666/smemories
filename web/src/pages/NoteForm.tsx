import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  MAX_PHOTOS,
  MAX_PHOTO_BYTES,
  PHOTO_TYPES,
  getNoteForm,
  submitNote,
  type NoteField,
  type NoteForm as Form,
} from "../api/publicNotes";
import { errorText } from "../auth/errors";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { PhotoTile } from "../components/ui/PhotoTile";
import { TextArea, TextField } from "../components/ui/TextField";

export const BUSY_RETRIES = 3; // 503 busy: the server took nothing, so waiting Retry-After (2 s if absent) and resending is safe
const SLOW_AFTER_MS = 6000;
const PHOTO_CODES = new Set([
  "invalid_image",
  "too_many_photos",
  "unsupported_media_type",
  "payload_too_large",
  "quota_exceeded",
]);

type Dead = "closed" | "full" | "gone";
const deadOf = (e: unknown): Dead | undefined =>
  !(e instanceof ApiError)
    ? undefined
    : e.code === "collection_closed"
      ? "closed"
      : e.code === "collection_full"
        ? "full"
        : e.code === "not_found"
          ? "gone" // revoked, unknown and malformed links are the same 404 on purpose: no oracle, no name
          : undefined;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// Text answers survive a reload while the form is open (never the photos). Storage may be blocked: every access is guarded.
// The link token is a bearer secret: the storage key holds only a hash of it (cyrb53), never the token.
function hashToken(s: string): string {
  let h1 = 0xdeadbeef;
  let h2 = 0x41c6ce57;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 2654435761);
    h2 = Math.imul(h2 ^ c, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36);
}
const draftKey = (token: string) => `smemories.note-draft.${hashToken(token)}`;
function loadDraft(token: string, fields: NoteField[]): Record<string, string> {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(draftKey(token)) ?? "{}");
    const out: Record<string, string> = {};
    for (const f of fields) {
      const v =
        typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>)[f.id] : "";
      out[f.id] = typeof v === "string" ? v.slice(0, f.max_length) : "";
    }
    return out;
  } catch {
    return Object.fromEntries(fields.map((f) => [f.id, ""]));
  }
}
function saveDraft(token: string, answers: Record<string, string> | null): boolean {
  try {
    if (answers) window.localStorage.setItem(draftKey(token), JSON.stringify(answers));
    else window.localStorage.removeItem(draftKey(token));
    return true;
  } catch {
    return false;
  }
}

/** The public page behind a shared link: no account, no cookie. The token in the address is the only credential. */
export function NoteForm() {
  const { token = "" } = useParams();
  const { t } = useTranslation();
  const form = useQuery({ queryKey: ["public-collect", token], queryFn: () => getNoteForm(token) });
  const [dead, setDead] = useState<Dead>();
  useEffect(() => {
    // a link is a secret: keep it out of search indexes and never send it in a Referer header
    const metas = [
      ["robots", "noindex"],
      ["referrer", "no-referrer"],
    ].map(([name, content]) => Object.assign(document.createElement("meta"), { name, content }));
    document.head.append(...metas);
    return () => metas.forEach((m) => m.remove());
  }, []);

  const d = dead ?? deadOf(form.error);
  return (
    <>
      <header className="app-header">
        <strong>{t("app.name")}</strong>
        <LanguageSwitcher />
      </header>
      <main className="note-main">
        {d ? (
          <DeadEnd kind={d} name={form.data?.owner.display_name} />
        ) : form.isPending ? (
          <p role="status">{t("auth.loading")}</p>
        ) : form.isError ? (
          <Banner
            variant="error"
            action={<Button onClick={() => void form.refetch()}>{t("auth.retry")}</Button>}
          >
            {errorText(t, form.error)}
          </Banner>
        ) : (
          <NoteFormBody token={token} form={form.data} onDead={setDead} />
        )}
      </main>
    </>
  );
}

function DeadEnd({ kind, name }: { kind: Dead; name?: string | undefined }) {
  const { t } = useTranslation();
  return (
    <EmptyState
      title={t(`note.${kind}.title`)}
      action={
        <Link className={buttonClass("ghost")} to="/">
          {t("note.what")}
        </Link>
      }
    >
      {t(`note.${kind}.body`, { name: name ?? t("note.theOwner") })}
    </EmptyState>
  );
}

function NoteFormBody({
  token,
  form,
  onDead,
}: {
  token: string;
  form: Form;
  onDead: (d: Dead) => void;
}) {
  const { t, i18n } = useTranslation();
  const lang = i18n.resolvedLanguage === "vi" ? "vi" : "en";
  const name = form.owner.display_name;
  const { fields } = form;

  const [answers, setAnswers] = useState(() => loadDraft(token, fields));
  const [photos, setPhotos] = useState<File[]>([]);
  const [photoNote, setPhotoNote] = useState<string>();
  const [missing, setMissing] = useState<Record<string, string>>({});
  const [focusTick, setFocusTick] = useState(0);
  const [sending, setSending] = useState(false);
  const [progress, setProgress] = useState(0);
  const [busy, setBusy] = useState(false);
  const [slow, setSlow] = useState(false);
  const [problem, setProblem] = useState<unknown>();
  const [sent, setSent] = useState<Record<string, string>>();
  const [saved, setSaved] = useState(true);
  const [trap, setTrap] = useState(""); // honeypot: people never see it, bots fill it
  const formRef = useRef<HTMLFormElement>(null);

  const previews = useMemo(() => photos.map((f) => URL.createObjectURL(f)), [photos]);
  useEffect(() => () => previews.forEach((u) => URL.revokeObjectURL(u)), [previews]);
  useEffect(() => {
    if (focusTick) formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }, [focusTick]);
  useEffect(() => {
    if (!sending) return;
    const id = setTimeout(() => setSlow(true), SLOW_AFTER_MS);
    return () => clearTimeout(id);
  }, [sending]);

  const setAnswer = (id: string, v: string) => {
    const next = { ...answers, [id]: v };
    setAnswers(next);
    setSaved(saveDraft(token, next));
    setMissing((m) => ({ ...m, [id]: "" }));
  };

  function addPhotos(e: ChangeEvent<HTMLInputElement>) {
    const picked = [...(e.target.files ?? [])];
    e.target.value = ""; // the same file can be picked again
    const keep: File[] = [];
    let note: string | undefined;
    for (const f of picked) {
      if (!PHOTO_TYPES.includes(f.type)) note = t("note.photoType", { name: f.name });
      else if (f.size > MAX_PHOTO_BYTES) note = t("note.photoSize", { name: f.name });
      else if (photos.length + keep.length >= MAX_PHOTOS)
        note = t("note.photoCount", { max: MAX_PHOTOS });
      else keep.push(f);
    }
    setPhotoNote(note);
    setProblem(undefined);
    if (keep.length) setPhotos([...photos, ...keep]);
  }

  async function send(withPhotos: boolean) {
    const body = Object.fromEntries(
      fields.map((f) => [f.id, (answers[f.id] ?? "").trim()] as const).filter(([, v]) => v),
    );
    const files = withPhotos ? photos : [];
    setSending(true);
    setSlow(false);
    setProblem(undefined);
    setProgress(0);
    for (let attempt = 0; ; attempt++) {
      try {
        await submitNote(token, body, files, setProgress, trap);
        saveDraft(token, null);
        setSent(body);
        break;
      } catch (e) {
        if (e instanceof ApiError && e.code === "busy" && attempt < BUSY_RETRIES) {
          setBusy(true);
          await sleep((e.retryAfter ?? 2) * 1000);
          setBusy(false);
          continue;
        }
        const dead = deadOf(e);
        if (dead) onDead(dead);
        else setProblem(e);
        break;
      }
    }
    setSending(false);
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    if (sending) return;
    const gaps = Object.fromEntries(
      fields
        .filter((f) => f.required && !(answers[f.id] ?? "").trim())
        .map((f) => [f.id, t("note.required")]),
    );
    setMissing(gaps);
    if (Object.keys(gaps).length) {
      setFocusTick((n) => n + 1);
      return;
    }
    void send(true);
  }

  if (sent) {
    const long = fields.find((f) => f.kind === "long_text" && sent[f.id]);
    const sign = sent.name;
    return (
      <section className="note-sent" aria-labelledby="note-sent-h">
        {long && (
          <blockquote className="note-card">
            <p>{sent[long.id]}</p>
            {sign && <footer>— {sign}</footer>}
          </blockquote>
        )}
        <h1 id="note-sent-h">
          {sign ? t("note.sent.titleName", { name: sign }) : t("note.sent.title")}
        </h1>
        <p>{t("note.sent.body", { name })}</p>
        <Button
          variant="ghost"
          onClick={() => {
            setSent(undefined);
            setAnswers(Object.fromEntries(fields.map((f) => [f.id, ""])));
            setPhotos([]);
          }}
        >
          {t("note.sent.another")}
        </Button>
        <p className="note-cta">
          {t("note.sent.cta")} <Link to="/register">{t("note.sent.make")}</Link>
        </p>
      </section>
    );
  }

  // The server names the field in the message of ERROR_MISSING_ANSWER / ERROR_INVALID_ANSWER, never the value.
  const answerCode =
    problem instanceof ApiError && ["missing_answer", "invalid_answer"].includes(problem.code);
  const badField = answerCode
    ? fields
        .filter((f) => (problem as ApiError).message.includes(f.id))
        .sort((a, b) => b.id.length - a.id.length)[0]?.id
    : undefined;
  const photoFail =
    problem instanceof ApiError && PHOTO_CODES.has(problem.code) && photos.length > 0;
  const fieldProblem = (id: string) =>
    missing[id] ||
    (badField === id
      ? t((problem as ApiError).code === "missing_answer" ? "note.required" : "note.invalidAnswer")
      : undefined);
  const generic = problem && !photoFail && !badField ? problem : undefined;
  const nFix = fields.filter((f) => fieldProblem(f.id)).length;

  return (
    <form
      ref={formRef}
      onSubmit={submit}
      noValidate
      className="note-form"
      aria-busy={sending || undefined}
    >
      <p className="note-eyebrow">{t("note.writingIn")}</p>
      <h1>{form.yearbook.title}</h1>
      <p className="note-privacy">{t("note.privacy", { name })}</p>

      {nFix > 0 && <Banner variant="error">{t("note.fix")}</Banner>}

      {fields.map((f) => {
        const Field = f.kind === "long_text" ? TextArea : TextField;
        const label = f.label[lang] + (f.required ? "" : ` ${t("note.optional")}`);
        return (
          <Field
            key={f.id}
            label={label}
            placeholder={f.hint[lang]}
            value={answers[f.id] ?? ""}
            onChange={(e) => setAnswer(f.id, e.target.value)}
            maxLength={f.max_length}
            counter
            required={f.required}
            disabled={sending}
            error={fieldProblem(f.id)}
            {...(f.kind === "long_text" ? { rows: 5 } : {})}
          />
        );
      })}

      <div className="note-photos">
        <p className="note-photos-head">
          <span>
            {t("note.photos")} <span className="note-muted">{t("note.optional")}</span>
          </span>
          <span className="note-muted">
            {t("note.photoTotal", { n: photos.length, max: MAX_PHOTOS })}
          </span>
        </p>
        <ul className="note-photo-grid">
          {photos.map((f, i) => (
            <li key={`${f.name}-${f.size}-${f.lastModified}-${i}`}>
              <PhotoTile
                src={previews[i] ?? ""}
                alt={t("note.photoAlt", { n: i + 1 })}
                removeLabel={t("note.removePhoto", { n: i + 1 })}
                disabled={sending}
                onRemove={() => setPhotos(photos.filter((_, j) => j !== i))}
              />
            </li>
          ))}
          {photos.length < MAX_PHOTOS && (
            <li>
              <label className="note-add-photo">
                {t("note.addPhoto")}
                <input
                  type="file"
                  multiple
                  accept={PHOTO_TYPES.join(",")}
                  className="visually-hidden"
                  disabled={sending}
                  onChange={addPhotos}
                />
              </label>
            </li>
          )}
        </ul>
        <small className="field-help">{t("note.photoHint")}</small>
        {photoNote && (
          <p role="alert" className="field-error">
            {photoNote}
          </p>
        )}
      </div>

      {photoFail && (
        <div role="alert" className="note-panel">
          <strong>{t("note.photoFail.title")}</strong>
          <p>{t("note.photoFail.body")}</p>
          <div className="note-panel-actions">
            <Button variant="primary" busy={sending} onClick={() => void send(true)}>
              {t("note.photoFail.retry")}
            </Button>
            <Button
              disabled={sending}
              onClick={() => {
                setPhotos([]);
                setProblem(undefined);
              }}
            >
              {t("note.photoFail.remove")}
            </Button>
            <Button
              disabled={sending}
              onClick={() => {
                setPhotos([]);
                void send(false);
              }}
            >
              {t("note.photoFail.without")}
            </Button>
          </div>
        </div>
      )}

      <div aria-hidden="true" className="note-trap">
        <input
          name="website"
          tabIndex={-1}
          autoComplete="off"
          value={trap}
          onChange={(e) => setTrap(e.target.value)}
        />
      </div>

      <p className="note-muted">{t("note.consent", { name })}</p>

      {generic !== undefined && (
        <p role="alert" className="error">
          {errorText(t, generic)}
        </p>
      )}
      {sending && (
        <div role="status" className="note-progress">
          <progress value={progress} max={1} aria-label={t("note.progress")} />
          <span>
            {busy ? t("note.busyRetry") : slow ? t("note.slow") : t("note.sending")}
            {slow && saved && ` ${t("note.saved")}`}
          </span>
        </div>
      )}
      <Button type="submit" variant="primary" busy={sending} className="note-send">
        {sending ? t("note.sending") : t("note.send")}
      </Button>
    </form>
  );
}
