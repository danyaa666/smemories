import type { CSSProperties, ReactNode } from "react";
import type { BookData, BookNote } from "../../api/book";
import { BookSignature, hasSignature } from "../BookSignature";
import { FitText } from "../fit";
import { Photo, Sheet, useBookEnv } from "../env";
import { designHeight } from "../sizes";
import type { Template } from "./index";
import dots from "./memory-dots.png";
import "./memory.css";

// Template "memory-book": the pastel Memory Book of design/canvas/temp1.html (US Letter, 816 x 1056 px), as components for the
// renderer of T-058. Four page kinds (D-17): cover, "All about me", "From a Friend" (one note per page), "Let's stay in touch".
// The design is a form to fill in by hand; here the lines hold the owner's profile and the friend's answers. Boxes keep the
// canvas's colours, radii, tilts and tape; texts shrink to fit their box (fit.tsx). Every user text goes through React as text.

const SOFT = "#5E5170";

const ICONS = {
  heart:
    "M12 20.5C5 15.5 2.5 12 2.5 8.6 2.5 5.9 4.6 4 7 4c2.1 0 3.9 1.3 5 3 1.1-1.7 2.9-3 5-3 2.4 0 4.5 1.9 4.5 4.6 0 3.4-2.5 6.9-9.5 11.9z",
  sparkle: "M12 3c.8 5 4 8.2 9 9-5 .8-8.2 4-9 9-.8-5-4-8.2-9-9 5-.8 8.2-4 9-9z",
  star: "M12 2.5l2.9 6.1 6.6.8-4.9 4.5 1.3 6.6L12 17.2l-5.9 3.3 1.3-6.6-4.9-4.5 6.6-.8z",
};
type Kind = keyof typeof ICONS;

function Icon({
  kind,
  size,
  stroke,
  style,
}: {
  kind: Kind;
  size: number;
  stroke: string;
  style?: CSSProperties;
}) {
  return (
    <svg
      style={style}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={stroke}
      strokeWidth={1.8}
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={ICONS[kind]} />
    </svg>
  );
}

/** A row of boxes keeps a minimum height (its basis in CSS) and shares what is left in proportion to its longest text: a 2000-character message gets the room, a one-line answer stays small. */
const rowWeight = (chars: number) => 0.25 + chars / 200;
/** A signed message card keeps room for its signature (88 px + 4 gap, see .mb-sig-foot) on top of the usual 104 px minimum. */
const MESSAGE_SIGNED_BASIS = 196;

/**
 * The dotted paper is one picture (memory-dots.png), not the canvas's tiled gradient: Chromium prints a tiled gradient as a
 * 72 dpi raster and Firefox as black (ADR 0003, finding 5). The PDF stores the picture once.
 */
function Page({ label, bg, children }: { label: string; bg: string; children: ReactNode }) {
  return (
    <Sheet label={label}>
      <div className="mb-page" style={{ backgroundColor: bg, backgroundImage: `url(${dots})` }}>
        {children}
      </div>
    </Sheet>
  );
}

function Footer({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const y = data.yearbook;
  return (
    <div className="mb-footer">
      <div className="mb-footer-title">{y.title}</div>
      <Icon kind="heart" size={14} stroke="#D9668C" />
      <div>{y.graduation_year !== null && t("bookPages.classOf", { year: y.graduation_year })}</div>
    </div>
  );
}

function Title({
  children,
  sub,
  icon,
  color,
}: {
  children: string;
  sub?: ReactNode;
  icon: Kind;
  color: string;
}) {
  return (
    <div>
      <div className="mb-title">
        <div className="disp mb-title-text">{children}</div>
        <Icon kind={icon} size={34} stroke={color} />
      </div>
      {typeof sub === "string" ? <div className="cute mb-title-sub">{sub}</div> : sub}
    </div>
  );
}

/** A label and one line of text on a rule. */
function Field({ fitKey, label, value }: { fitKey: string; label: string; value: string }) {
  return (
    <div className="mb-field">
      <div className="mb-label">{label}</div>
      <FitText fitKey={fitKey} className="mb-value" style={{ fontSize: 18 }}>
        {value}
      </FitText>
    </div>
  );
}

/** A pastel box with an icon and a title, and ruled lines that hold the text; the rules follow the font size (1lh). */
function Ruled({
  fitKey,
  title,
  bg,
  icon,
  iconColor,
  grow = 1,
  side,
  foot,
  children,
}: {
  fitKey: string;
  title: string;
  bg: string;
  icon: Kind;
  iconColor: string;
  grow?: number;
  side?: ReactNode;
  /** Below the text, inside the card (the friend's signature): the text box shrinks by its height. */
  foot?: ReactNode;
  children: string;
}) {
  return (
    <div className="mb-ruled" style={{ background: bg, flexGrow: grow }}>
      <div className="mb-ruled-head">
        <Icon kind={icon} size={20} stroke={iconColor} />
        <div className="disp mb-ruled-title">{title}</div>
      </div>
      <div className="mb-ruled-body">
        <FitText fitKey={fitKey} className="mb-lines" style={{ fontSize: 20 }}>
          {children}
        </FitText>
        {side}
      </div>
      {foot}
    </div>
  );
}

function Polaroid({
  id,
  className,
  style,
}: {
  id?: string | null;
  className: string;
  style?: CSSProperties;
}) {
  return (
    <div className={`mb-polaroid ${className}`} style={style}>
      <Photo id={id} className="mb-polaroid-photo" />
    </div>
  );
}

function Cover({ data }: { data: BookData }) {
  const { t, size } = useBookEnv();
  const y = data.yearbook;
  const rings = Math.floor((designHeight(size) - 64 - 124) / 62) + 1;
  const line = [
    y.graduation_year !== null && t("bookPages.classOf", { year: y.graduation_year }),
    y.class_name,
    y.school_name,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <Page label="cover" bg="#E6DEFA">
      <div className="mb-spine" />
      {Array.from({ length: rings }, (_, i) => (
        <div key={i} className="mb-ring" style={{ top: 64 + i * 62 }} />
      ))}
      <div className="mb-cover-head">
        <div className="cute" style={{ fontSize: 32, color: SOFT }}>
          {t("bookPages.memory.belongs")}
        </div>
        <FitText
          fitKey="cover:owner"
          className="disp mb-cover-owner"
          style={{ fontSize: 62 }}
          minScale={0.4}
        >
          {t("bookPages.memory.owner", { name: data.profile.full_name })}
        </FitText>
        <FitText
          fitKey="cover:title"
          className="disp mb-cover-title"
          style={{ fontSize: 100 }}
          minScale={0.35}
        >
          {y.title}
        </FitText>
        {line && (
          <FitText fitKey="cover:class" className="mb-cover-class" style={{ fontSize: 14 }}>
            {line}
          </FitText>
        )}
      </div>
      <Polaroid
        id={y.cover_media_id}
        className="mb-cover-photo"
        style={{
          left: 190,
          top: 410,
          width: 370,
          padding: 14,
          borderRadius: 20,
          transform: "rotate(-3deg)",
        }}
      />
      <div
        className="mb-tape"
        style={{ left: 310, top: 394, width: 140, height: 28, transform: "rotate(-5deg)" }}
      />
      <div className="mb-sticker" style={{ left: 560, top: 680 }}>
        <div className="cute mb-sticker-text">{t("bookPages.memory.sticker")}</div>
      </div>
      <Icon
        kind="heart"
        size={40}
        stroke="#D9668C"
        style={{ position: "absolute", left: 120, top: 440, transform: "rotate(-12deg)" }}
      />
      <Icon
        kind="sparkle"
        size={46}
        stroke="#8E6CCB"
        style={{ position: "absolute", left: 640, top: 420 }}
      />
      <Icon
        kind="star"
        size={30}
        stroke="#E09A2D"
        style={{ position: "absolute", left: 600, top: 560, transform: "rotate(15deg)" }}
      />
      <Icon
        kind="sparkle"
        size={30}
        stroke="#8E6CCB"
        style={{ position: "absolute", left: 130, top: 800 }}
      />
      <Icon
        kind="heart"
        size={26}
        stroke="#8E6CCB"
        style={{ position: "absolute", left: 700, top: 900, transform: "rotate(14deg)" }}
      />
      <Icon
        kind="star"
        size={26}
        stroke="#E09A2D"
        style={{ position: "absolute", left: 150, top: 690, transform: "rotate(-10deg)" }}
      />
      <div className="mb-cover-foot">
        {y.motto ? (
          <FitText fitKey="cover:motto" className="cute mb-cover-motto" style={{ fontSize: 28 }}>
            {y.motto}
          </FitText>
        ) : (
          <div />
        )}
        {y.graduation_year !== null && <div className="mb-cover-year">{y.graduation_year}</div>}
      </div>
    </Page>
  );
}

function Profile({ data }: { data: BookData }) {
  const { t } = useBookEnv();
  const p = data.profile;
  const y = data.yearbook;
  return (
    <Page label="profile" bg="#FFFBF6">
      <div className="mb-flow">
        <Title sub={t("bookPages.memory.ownerOf")} icon="sparkle" color="#8E6CCB">
          {t("bookPages.aboutMe")}
        </Title>
        <div className="mb-head-row">
          <Polaroid id={p.photo_media_id} className="mb-profile-photo" />
          <div className="mb-fields" style={{ height: 280 }}>
            <Field
              fitKey="profile:name"
              label={t("bookPages.memory.fullName")}
              value={p.full_name}
            />
            <Field
              fitKey="profile:nickname"
              label={t("bookPages.memory.nickname")}
              value={p.nickname}
            />
            <Field
              fitKey="profile:class"
              label={t("bookPages.memory.class")}
              value={y.class_name}
            />
            <Field
              fitKey="profile:school"
              label={t("bookPages.memory.school")}
              value={y.school_name}
            />
            <Field
              fitKey="profile:year"
              label={t("bookPages.memory.graduation")}
              value={y.graduation_year === null ? "" : String(y.graduation_year)}
            />
          </div>
        </div>
        <div
          className="mb-grid2"
          style={{ flexGrow: rowWeight(Math.max(p.hobbies.length, p.future_plans.length)) }}
        >
          <Ruled
            fitKey="profile:hobbies"
            title={t("bookPages.hobbies")}
            bg="#FFDCCB"
            icon="star"
            iconColor="#E09A2D"
          >
            {p.hobbies}
          </Ruled>
          <Ruled
            fitKey="profile:future"
            title={t("bookPages.futurePlans")}
            bg="#D2EEDF"
            icon="heart"
            iconColor="#D9668C"
          >
            {p.future_plans}
          </Ruled>
        </div>
        <div className="mb-motto" style={{ flexGrow: rowWeight(p.quote.length) }}>
          <Icon kind="sparkle" size={22} stroke="#8E6CCB" />
          <div className="disp mb-motto-label">{t("bookPages.quote")}</div>
          <FitText fitKey="profile:quote" className="mb-motto-text" style={{ fontSize: 20 }}>
            {p.quote}
          </FitText>
        </div>
      </div>
      <div className="mb-foot">
        <Footer data={data} />
      </div>
    </Page>
  );
}

const PHOTO_TILT = ["rotate(3deg)", "rotate(-3deg)"];

function FriendPage({ note, number, data }: { note: BookNote; number: number; data: BookData }) {
  const { t } = useBookEnv();
  const a = note.answers;
  const key = (f: string) => `note:${note.id}:${f}`;
  const box = (
    f: "how_we_met" | "first_impression" | "best_memory" | "wish" | "advice",
    bg: string,
    icon: Kind,
    color: string,
  ) =>
    a[f] ? (
      <Ruled
        key={f}
        fitKey={key(f)}
        title={t(`bookPages.fields.${f}`)}
        bg={bg}
        icon={icon}
        iconColor={color}
      >
        {a[f]}
      </Ruled>
    ) : null;
  const rows: { fields: readonly string[]; nodes: ReactNode[] }[] = [
    {
      fields: ["how_we_met", "first_impression"] as const,
      nodes: [
        box("how_we_met", "#FFDCCB", "star", "#E09A2D"),
        box("first_impression", "#D2EEDF", "sparkle", "#8E6CCB"),
      ],
    },
    {
      fields: ["best_memory"] as const,
      nodes: [box("best_memory", "#D3E8F8", "heart", "#D9668C")],
    },
    {
      fields: ["wish", "advice"] as const,
      nodes: [
        box("wish", "#FFF1B5", "star", "#E09A2D"),
        box("advice", "#E6DEFA", "sparkle", "#8E6CCB"),
      ],
    },
  ].filter((r) => r.nodes.some(Boolean));
  const extra = note.photo_ids.slice(1, 3);
  const signed = hasSignature(note.signature);
  return (
    <Page label="notes" bg="#FFFBF6">
      <div className="mb-flow">
        <Title
          icon="heart"
          color="#D9668C"
          sub={
            <FitText
              fitKey={`sub:${note.id}`}
              className="cute mb-title-sub mb-title-line"
              style={{ fontSize: 26 }}
            >
              {data.profile.full_name &&
                t("bookPages.memory.forYou", { name: data.profile.full_name })}
            </FitText>
          }
        >
          {t("bookPages.memory.friendTitle")}
        </Title>
        <div className="mb-head-row">
          {note.photo_ids[0] ? (
            <Polaroid id={note.photo_ids[0]} className="mb-friend-photo" />
          ) : (
            <div className="mb-stick">
              <Icon kind="heart" size={28} stroke="#D9668C" />
              <div className="cute mb-stick-text">{t("bookPages.memory.stickPhoto")}</div>
            </div>
          )}
          <div className="mb-fields" style={{ height: 220 }}>
            <Field fitKey={key("name")} label={t("bookPages.fields.name")} value={a.name ?? ""} />
            <Field
              fitKey={key("nickname")}
              label={t("bookPages.fields.nickname")}
              value={a.nickname ?? ""}
            />
            <Field
              fitKey={key("relationship")}
              label={t("bookPages.fields.relationship")}
              value={a.relationship ?? ""}
            />
          </div>
        </div>
        <div className="mb-grow mb-rows">
          {rows.map((r, i) => (
            <div
              key={i}
              className="mb-row"
              style={{ flexGrow: rowWeight(Math.max(...r.fields.map((f) => (a[f] ?? "").length))) }}
            >
              {r.nodes}
            </div>
          ))}
          {a.message && (
            <div
              className="mb-row"
              style={{
                flexGrow: rowWeight(a.message.length),
                ...(signed && { flexBasis: MESSAGE_SIGNED_BASIS }),
              }}
            >
              <Ruled
                fitKey={key("message")}
                title={t("bookPages.fields.message")}
                bg="#E6DEFA"
                icon="heart"
                iconColor="#D9668C"
                foot={
                  signed && (
                    <div className="mb-sig-foot">
                      <BookSignature
                        value={note.signature}
                        label={t("note.signature.of", { name: a.name ?? "" })}
                        className="mb-sig"
                      />
                    </div>
                  )
                }
                side={
                  extra.length > 0 && (
                    <div className="mb-side">
                      {extra.map((id, i) => (
                        <Polaroid
                          key={id}
                          id={id}
                          className="mb-side-photo"
                          style={{ transform: PHOTO_TILT[i] }}
                        />
                      ))}
                    </div>
                  )
                }
              >
                {a.message}
              </Ruled>
            </div>
          )}
        </div>
      </div>
      <div className="mb-no">
        {t("bookPages.memory.no")} <span className="mb-no-n">{number}</span>
      </div>
      <div className="mb-foot">
        <Footer data={data} />
      </div>
    </Page>
  );
}

function Notes({ notes, data, first }: { notes: BookNote[]; data: BookData; first: number }) {
  return (
    <>
      {notes.map((n, i) => (
        <FriendPage key={n.id} note={n} number={first + i + 1} data={data} />
      ))}
    </>
  );
}

/** "Let's stay in touch": the contact table and the reunion pact, left blank to be written by hand (the app holds no phone numbers). */
function Back({ data }: { data: BookData }) {
  const { t, size } = useBookEnv();
  const rows = 16 + Math.floor((designHeight(size) - 1056) / 34);
  const head = [
    ["#", 36],
    [t("bookPages.memory.colName"), 210],
    [t("bookPages.memory.colPhone"), 150],
    [t("bookPages.memory.colSocial"), 0],
    [t("bookPages.memory.colBirthday"), 110],
  ] as const;
  return (
    <Page label="back" bg="#FFFBF6">
      <div className="mb-flow">
        <Title sub={t("bookPages.memory.touchSub")} icon="heart" color="#D9668C">
          {t("bookPages.memory.touchTitle")}
        </Title>
        <table className="mb-table">
          <thead>
            <tr>
              {head.map(([h, w], i) => (
                <th key={i} style={w ? { width: w } : undefined}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: rows }, (_, i) => (
              <tr key={i}>
                <td>{i + 1}</td>
                <td />
                <td />
                <td />
                <td />
              </tr>
            ))}
          </tbody>
        </table>
        <div className="mb-pact">
          <div className="mb-pact-text">
            <div className="mb-pact-head">
              <Icon kind="star" size={22} stroke="#E09A2D" />
              <div className="disp" style={{ fontSize: 22, fontWeight: 600 }}>
                {t("bookPages.memory.pact")}
              </div>
            </div>
            <div className="cute" style={{ fontSize: 22, lineHeight: 1.1 }}>
              {t("bookPages.memory.pactSub")}
            </div>
          </div>
          <div className="mb-fields" style={{ flexGrow: 1 }}>
            <div className="mb-field">
              <div className="mb-label">{t("bookPages.memory.date")}</div>
              <div className="mb-value" />
            </div>
            <div className="mb-field">
              <div className="mb-label">{t("bookPages.memory.place")}</div>
              <div className="mb-value" />
            </div>
          </div>
        </div>
      </div>
      <Icon
        kind="sparkle"
        size={30}
        stroke="#8E6CCB"
        style={{ position: "absolute", left: 700, top: 66 }}
      />
      <div className="mb-foot">
        <Footer data={data} />
      </div>
    </Page>
  );
}

export const memoryBook: Template = {
  id: "memory-book",
  notesPerPage: 1,
  Cover,
  Profile,
  Notes,
  Back,
};
