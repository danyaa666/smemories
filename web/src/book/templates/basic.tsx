import type { BookData, BookNote } from "../../api/book";
import { FitText } from "../fit";
import { Photo, Sheet, useBookEnv } from "../env";
import type { Template } from "./index";

// The built-in layout that proves the pipeline until the designed templates arrive (T-059, T-060). Drawn in the 816 px box.
// Every string a friend or the owner wrote goes through React as text; nothing here builds HTML.

/** The answers printed after the name and message, in the order of docs/note-fields.md. */
const PER_PAGE = 4;
const EXTRA_FIELDS = ["how_we_met", "first_impression", "best_memory", "wish", "advice"] as const;

function Cover({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const y = data.yearbook;
  const sub = [y.school_name, y.class_name].filter(Boolean).join(" · ");
  return (
    <Sheet label="cover">
      <div className="bk-cover">
        <Photo id={y.cover_media_id} className="bk-cover-photo" />
        <FitText fitKey="cover:title" className="bk-title" style={{ fontSize: 56 }}>
          {y.title}
        </FitText>
        {sub && <p className="bk-sub">{sub}</p>}
        {y.graduation_year !== null && (
          <p className="bk-sub">{t("bookPages.classOf", { year: y.graduation_year })}</p>
        )}
        {y.motto && (
          <FitText fitKey="cover:motto" className="bk-motto" style={{ fontSize: 24 }}>
            {y.motto}
          </FitText>
        )}
      </div>
    </Sheet>
  );
}

function ProfileBox({ id, label, text }: { id: string; label: string; text: string }) {
  if (!text) return null;
  return (
    <div className="bk-box">
      <h3>{label}</h3>
      <FitText fitKey={`profile:${id}`} className="bk-box-text" style={{ fontSize: 20 }}>
        {text}
      </FitText>
    </div>
  );
}

function Profile({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const p = data.profile;
  return (
    <Sheet label="profile">
      <div className="bk-profile">
        <h2>{t("bookPages.aboutMe")}</h2>
        <div className="bk-profile-head">
          <Photo id={p.photo_media_id} className="bk-profile-photo" />
          <div>
            <p className="bk-name">{p.full_name}</p>
            {p.nickname && <p className="bk-sub">{p.nickname}</p>}
          </div>
        </div>
        <ProfileBox id="quote" label={t("bookPages.quote")} text={p.quote} />
        <ProfileBox id="hobbies" label={t("bookPages.hobbies")} text={p.hobbies} />
        <ProfileBox id="future" label={t("bookPages.futurePlans")} text={p.future_plans} />
      </div>
    </Sheet>
  );
}

function Note({ note }: { note: BookNote }) {
  const { t } = useBookEnv();
  const a = note.answers;
  const head = [a.name, a.nickname && `(${a.nickname})`].filter(Boolean).join(" ");
  return (
    <article className="bk-note">
      <FitText fitKey={`note:${note.id}`} className="bk-note-text" style={{ fontSize: 16 }}>
        <p className="bk-note-name">
          {head}
          {a.relationship && <span className="bk-rel"> · {a.relationship}</span>}
        </p>
        {a.message && <p className="bk-note-body">{a.message}</p>}
        {EXTRA_FIELDS.filter((f) => a[f]).map((f) => (
          <p key={f} className="bk-note-body">
            <b>{t(`bookPages.fields.${f}`)}</b> {a[f]}
          </p>
        ))}
      </FitText>
      {note.photo_ids.length > 0 && (
        <div className="bk-note-photos">
          {note.photo_ids.map((id) => (
            <Photo key={id} id={id} className="bk-note-photo" />
          ))}
        </div>
      )}
    </article>
  );
}

function Notes({ notes }: { notes: BookNote[] }) {
  const { t } = useBookEnv();
  return (
    <Sheet label="notes">
      <div className="bk-notes">
        <h2>{t("bookPages.notes")}</h2>
        <div
          className="bk-note-list"
          style={{ gridTemplateRows: `repeat(${PER_PAGE}, minmax(0, 1fr))` }}
        >
          {notes.map((n) => (
            <Note key={n.id} note={n} />
          ))}
        </div>
      </div>
    </Sheet>
  );
}

function Back({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  return (
    <Sheet label="back">
      <div className="bk-back">
        <p className="bk-title-small">{data.yearbook.title}</p>
        <p className="bk-sub">{t("bookPages.thanks")}</p>
      </div>
    </Sheet>
  );
}

export const basic: Template = {
  id: "basic",
  notesPerPage: PER_PAGE,
  Cover,
  Profile,
  Notes,
  Back,
};
