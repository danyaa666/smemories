import type { CSSProperties, ReactNode } from "react";
import type { BookData, BookNote } from "../../api/book";
import { BookSignature, hasSignature } from "../BookSignature";
import { useDesign } from "../design";
import { FitText } from "../fit";
import { Photo, Sheet, useBookEnv } from "../env";
import { designHeight } from "../sizes";
import type { Template } from "./index";
import "./navy.css";

// Template "navy-classic": the navy and gold canvas of design/canvas/temp2.html (US Letter, 816 x 1056 px), as components for the
// renderer of T-058. Four page kinds (D-17): the cover, the profile (the welcome-letter layout with an oval photo), the notes page
// (the "Autographs" page: four friends per page, two columns of two) and the back page (the cover's frame). Every user text goes
// through React as text; texts that do not fit shrink (fit.tsx). Sizes, colours and positions are the canvas's (see navy.css).

// The colours are the custom properties of navy.css (a theme sets them on .nc-page); the SVG pieces read them as presentation attributes.
const GOLD = "var(--nc-gold)";
const DARK_GOLD = "var(--nc-bronze)";

/** The page: the theme and the font set of the book's design (the first of each when it has none) are attributes of the element navy.css reads. */
function Page({ tone, children }: { tone: "navy" | "cream"; children: ReactNode }) {
  const { theme, fonts } = useDesign();
  return (
    <div
      className={`nc-page nc-${tone}`}
      data-theme={theme || undefined}
      data-fonts={fonts || undefined}
    >
      {children}
    </div>
  );
}

/**
 * A fixed text of the template, which the owner may replace (design texts, E09). The template's own wording keeps the plain markup it always had; a
 * replacement is drawn in a box of the same one-line size (`fitClass`) that shrinks long text and reports it like any other box (fit.tsx).
 */
function Own({
  id,
  fallback,
  fitKey,
  className,
  fitClass,
  style,
}: {
  id: string;
  fallback: string;
  fitKey: string;
  className: string;
  fitClass: string;
  style?: CSSProperties;
}) {
  const shown = useDesign().text(id, fallback);
  if (shown === fallback)
    return (
      <div className={className} style={style}>
        {fallback}
      </div>
    );
  return (
    <FitText fitKey={fitKey} className={`${className} ${fitClass}`} style={style} minScale={0.35}>
      {shown}
    </FitText>
  );
}

/** Two thin gold frames, as on the cover and the autographs page of the canvas. */
function Frame({ outer, inner }: { outer: number; inner: number }) {
  return (
    <>
      <div className="nc-frame" style={{ inset: outer }} />
      <div className="nc-frame" style={{ inset: inner }} />
    </>
  );
}

/** The thin rule with a diamond in the middle. */
function Divider({ width, color }: { width: number; color: string }) {
  const m = width / 2;
  return (
    <svg width={width} height={14} viewBox={`0 0 ${width} 14`} aria-hidden="true">
      <path d={`M0 7h${m - 14}M${m + 14} 7h${m - 14}`} stroke={color} strokeWidth={1} />
      <path d={`M${m} 1l6 6-6 6-6-6z`} fill="none" stroke={color} strokeWidth={1.2} />
      <circle cx={m} cy={7} r={1.6} fill={color} />
    </svg>
  );
}

function Cap() {
  return (
    <svg
      width={64}
      height={64}
      viewBox="0 0 24 24"
      fill="none"
      stroke={GOLD}
      strokeWidth={1}
      strokeLinejoin="round"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M2 9l10-5 10 5-10 5z" />
      <path d="M6 11v5c0 1.5 3 3 6 3s6-1.5 6-3v-5" />
      <path d="M22 9v6" />
    </svg>
  );
}

/** Small letter-spaced capitals over a title, then the diamond rule: the head of the profile and the notes page. */
function Head({
  page,
  eyebrow,
  title,
  top,
}: {
  /** The section: names the design texts (`<page>.eyebrow`, `<page>.heading`) and the fit boxes. */
  page: "profile" | "notes";
  eyebrow: string;
  title: string;
  top: number;
}) {
  return (
    <div className="nc-head" style={{ top }}>
      <Own
        id={`${page}.eyebrow`}
        fallback={eyebrow}
        fitKey={`${page}:eyebrow`}
        className="caps nc-eyebrow"
        fitClass="nc-eyebrow-fit"
        style={{ fontSize: 12 }}
      />
      <Own
        id={`${page}.heading`}
        fallback={title}
        fitKey={`${page}:heading`}
        className="serif nc-title"
        fitClass="nc-title-fit"
        style={{ fontSize: 68 }}
      />
      <Divider width={220} color={DARK_GOLD} />
    </div>
  );
}

function Cover({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const y = data.yearbook;
  return (
    <Sheet label="cover">
      <Page tone="navy">
        <Frame outer={36} inner={44} />
        <div className="nc-cover-top">
          <Cap />
          {y.school_name && (
            <FitText
              fitKey="cover:school"
              className="caps nc-line nc-school"
              style={{ fontSize: 16 }}
              minScale={0.3}
            >
              {y.school_name}
            </FitText>
          )}
          <Divider width={260} color={GOLD} />
          {y.graduation_year !== null ? (
            <>
              <div className="serif nc-class-of">{t("bookPages.navy.classOf")}</div>
              <div className="serif nc-year">{y.graduation_year}</div>
              <Divider width={260} color={GOLD} />
              <FitText
                fitKey="cover:title"
                className="caps nc-line nc-sub"
                style={{ fontSize: 14 }}
                minScale={0.3}
              >
                {y.title}
              </FitText>
            </>
          ) : (
            <>
              <FitText
                fitKey="cover:title"
                className="serif nc-line nc-title-only"
                style={{ fontSize: 64 }}
                minScale={0.35}
              >
                {y.title}
              </FitText>
              <Divider width={260} color={GOLD} />
            </>
          )}
        </div>
        <div className="nc-cover-foot">
          {y.motto && (
            <FitText
              fitKey="cover:motto"
              className="serif nc-line nc-motto"
              style={{ fontSize: 24 }}
              minScale={0.35}
            >
              {y.motto}
            </FitText>
          )}
          {y.class_name && (
            <FitText
              fitKey="cover:class"
              className="caps nc-line nc-gold-caps"
              style={{ fontSize: 11 }}
              minScale={0.35}
            >
              {y.class_name}
            </FitText>
          )}
        </div>
      </Page>
    </Sheet>
  );
}

/**
 * The welcome letter: the owner's answers as its paragraphs (hobbies, plans, favourite quote), the oval portrait at the right,
 * the name as the signature. A paragraph is as tall as its text needs at the designed size (an estimate: 38 characters per line,
 * 34 px per line); when the estimates do not fit above the signature they shrink in proportion, and the font shrinks to fit the box.
 * The heights are computed here and set in px, never left to flex-shrink: in the PRINTED page the flex layout of a letter with long
 * fields differed from the screen's and pushed the quote and the signature off the page (T-060 QA).
 */
const paraHeight = (text: string) =>
  24 + 34 * text.split("\n").reduce((n, l) => n + Math.max(1, Math.ceil(l.length / 38)), 0);
const LETTER_TOP = 270;
const LETTER_BOTTOM = 96;
const LETTER_GAP = 18;
const SIGN_H = 80; // the name (56), its line (20) and the gap between them (2), rounded up

function Para({
  fitKey,
  label,
  text,
  height,
  italic,
}: {
  fitKey: string;
  label: string;
  text: string;
  height: number;
  italic?: boolean;
}) {
  return (
    <div className="nc-para" style={{ height }}>
      <div className="caps nc-para-label">{label}</div>
      <FitText
        fitKey={fitKey}
        className={`serif nc-para-text${italic ? " nc-italic" : ""}`}
        style={{ fontSize: 22 }}
      >
        {text}
      </FitText>
    </div>
  );
}

function Profile({ data }: { data: BookData }) {
  const { t, size } = useBookEnv();
  const p = data.profile;
  const y = data.yearbook;
  const signLine = [p.nickname, y.class_name, y.school_name].filter(Boolean).join(" · ");
  const paras = [
    { fitKey: "profile:hobbies", label: t("bookPages.hobbies"), text: p.hobbies },
    { fitKey: "profile:future", label: t("bookPages.futurePlans"), text: p.future_plans },
    { fitKey: "profile:quote", label: t("bookPages.quote"), text: p.quote, italic: true },
  ].filter((q) => q.text);
  const room = designHeight(size) - LETTER_TOP - LETTER_BOTTOM - SIGN_H - LETTER_GAP * paras.length;
  const scale = Math.min(1, room / paras.reduce((n, q) => n + paraHeight(q.text), 0));
  return (
    <Sheet label="profile">
      <Page tone="cream">
        <Head
          page="profile"
          eyebrow={t("bookPages.navy.profileEyebrow")}
          title={t("bookPages.aboutMe")}
          top={72}
        />
        <div className="nc-letter">
          <div className="nc-letter-text">
            {paras.map((q) => (
              <Para key={q.fitKey} {...q} height={Math.floor(paraHeight(q.text) * scale)} />
            ))}
            <div className="nc-sign">
              <FitText
                fitKey="profile:name"
                className="serif nc-line nc-signature"
                style={{ fontSize: 40 }}
                minScale={0.3}
              >
                {p.full_name}
              </FitText>
              {signLine && (
                <FitText
                  fitKey="profile:line"
                  className="caps nc-line nc-sign-line"
                  style={{ fontSize: 12 }}
                  minScale={0.35}
                >
                  {signLine}
                </FitText>
              )}
            </div>
          </div>
          <div className="nc-letter-photo">
            <Photo id={p.photo_media_id} className="nc-oval" />
            {y.graduation_year !== null && (
              <div className="serif nc-caption">
                {t("bookPages.classOf", { year: y.graduation_year })}
              </div>
            )}
          </div>
        </div>
        <div className="nc-footer">
          <div className="caps nc-footer-class">
            {y.graduation_year !== null && t("bookPages.classOf", { year: y.graduation_year })}
          </div>
          <div className="caps nc-footer-title">{y.title}</div>
        </div>
      </Page>
    </Sheet>
  );
}

/**
 * One friend, in the way of a signed autograph: the message on ruled lines, the name below it as the signature. A friend's drawn
 * signature (T-097) sits at the right end of the name line, in the height the name and its line already take (58 px), so the
 * note, its text fit and the four-per-page capacity are unchanged; only the name's width is smaller. No drawn signature: as before.
 */
function Autograph({ note }: { note: BookNote }) {
  const { t } = useBookEnv();
  const a = note.answers;
  const key = (f: string) => `note:${note.id}:${f}`;
  const name = (
    <>
      <FitText
        fitKey={key("name")}
        className="serif nc-line nc-signature"
        style={{ fontSize: 28 }}
        minScale={0.35}
      >
        {a.name ?? ""}
      </FitText>
      {a.relationship && (
        <FitText
          fitKey={key("relationship")}
          className="caps nc-line nc-sign-line"
          style={{ fontSize: 10 }}
          minScale={0.35}
        >
          {a.relationship}
        </FitText>
      )}
    </>
  );
  return (
    <div className="nc-note">
      <FitText fitKey={key("message")} className="serif nc-msg" style={{ fontSize: 19 }}>
        {a.message ?? ""}
      </FitText>
      {hasSignature(note.signature) ? (
        <div className="nc-note-sign nc-signed">
          <div className="nc-sign nc-signed-name">{name}</div>
          <BookSignature
            value={note.signature}
            label={t("note.signature.of", { name: a.name ?? "" })}
            className="nc-sig"
          />
        </div>
      ) : (
        <div className="nc-sign nc-note-sign">{name}</div>
      )}
    </div>
  );
}

function Notes({ notes, data }: { notes: BookNote[]; data: BookData }) {
  const { t } = useBookEnv();
  const y = data.yearbook;
  return (
    <Sheet label="notes">
      <Page tone="cream">
        <Frame outer={28} inner={34} />
        <Head
          page="notes"
          eyebrow={t("bookPages.navy.autographsEyebrow")}
          title={t("bookPages.navy.autographs")}
          top={76}
        />
        <div className="nc-grid">
          {notes.map((n) => (
            <Autograph key={n.id} note={n} />
          ))}
        </div>
        <div className="nc-farewell">
          <Divider width={220} color={DARK_GOLD} />
          <div className="serif nc-farewell-text">
            {y.graduation_year !== null
              ? t("bookPages.navy.farewellYear", { year: y.graduation_year })
              : t("bookPages.navy.farewell")}
          </div>
          {y.school_name && (
            <FitText
              fitKey="notes:school"
              className="caps nc-line nc-gold-caps nc-dark"
              style={{ fontSize: 11 }}
              minScale={0.35}
            >
              {y.school_name}
            </FitText>
          )}
        </div>
      </Page>
    </Sheet>
  );
}

/** The back page: the cover's frame and navy, the book's title and thanks. */
function Back({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const y = data.yearbook;
  return (
    <Sheet label="back">
      <Page tone="navy">
        <Frame outer={36} inner={44} />
        <div className="nc-back">
          <Cap />
          {y.school_name && (
            <FitText
              fitKey="back:school"
              className="caps nc-line nc-school"
              style={{ fontSize: 16 }}
              minScale={0.3}
            >
              {y.school_name}
            </FitText>
          )}
          <Divider width={260} color={GOLD} />
          <FitText
            fitKey="back:title"
            className="serif nc-line nc-title-only"
            style={{ fontSize: 48 }}
            minScale={0.35}
          >
            {y.title}
          </FitText>
          <Divider width={260} color={GOLD} />
          <Own
            id="back.thanks"
            fallback={t("bookPages.thanks")}
            fitKey="back:thanks"
            className="serif nc-thanks"
            fitClass="nc-thanks-fit"
            style={{ fontSize: 26 }}
          />
        </div>
        {y.graduation_year !== null && (
          <div className="nc-cover-foot">
            <div className="caps nc-line nc-gold-caps" style={{ fontSize: 11 }}>
              {t("bookPages.classOf", { year: y.graduation_year })}
            </div>
          </div>
        )}
      </Page>
    </Sheet>
  );
}

export const navyClassic: Template = {
  id: "navy-classic",
  notesPerPage: 4,
  // This design has no place for the cover photo or the friends' photos: do not fetch them.
  photos: { cover: false, notes: false },
  design: { themes: ["navy", "burgundy", "forest"], fonts: ["classic", "modern"] },
  swatch: {
    root: "nc-page",
    chips: ["--nc-dark", "--nc-gold", "--nc-light", "--nc-bronze"],
    serif: "serif",
  },
  // the locale key of each text's own wording (the customise screen shows it as the placeholder); design.test.ts pins the ids to the manifest
  textDefaults: {
    "profile.eyebrow": "bookPages.navy.profileEyebrow",
    "profile.heading": "bookPages.aboutMe",
    "notes.eyebrow": "bookPages.navy.autographsEyebrow",
    "notes.heading": "bookPages.navy.autographs",
    "back.thanks": "bookPages.thanks",
  },
  Cover,
  Profile,
  Notes,
  Back,
};
