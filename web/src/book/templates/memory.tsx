import type { CSSProperties, ReactNode } from "react";
import type { BookData, BookNote } from "../../api/book";
import { BookSignature, hasSignature } from "../BookSignature";
import { FitText } from "../fit";
import { useDesign } from "../design";
import { Photo, Sheet, useBookEnv } from "../env";
import {
  Editable,
  hasLayoutBg,
  LayoutLayer,
  useLayoutBg,
  type TemplateLayout,
} from "../layout/runtime";
import { designHeight } from "../sizes";
import type { Template } from "./index";
import dots from "./memory-dots.png";
import "./memory.css";

// Template "memory-book": the pastel Memory Book of design/canvas/temp1.html (US Letter, 816 x 1056 px), as components for the
// renderer of T-058. Four page kinds (D-17): cover, "All about me", "From a Friend" (one note per page), "Let's stay in touch".
// The design is a form to fill in by hand; here the lines hold the owner's profile and the friend's answers. Boxes keep the
// canvas's colours, radii, tilts and tape; texts shrink to fit their box (fit.tsx). Every user text goes through React as text.

const SOFT = "var(--mb-soft)";

/** The files of the `cover.pattern` choices (manifest ids); a choice this build does not know draws the first, today's. */
const PATTERNS: Record<string, string | null> = { dots, plain: null };

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
      style={{ stroke, ...style }}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
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
/** A signed message card is never shorter than its signature (88 px) plus the card's head and padding. */
const MESSAGE_SIGNED_BASIS = 150;

/**
 * The dotted paper is one picture (memory-dots.png), not the canvas's tiled gradient: Chromium prints a tiled gradient as a
 * 72 dpi raster and Firefox as black (ADR 0003, finding 5). The PDF stores the picture once.
 */
function Page({
  label,
  bg,
  pattern = dots,
  children,
}: {
  label: string;
  bg: string;
  /** The paper picture; null for none (the `plain` choice). */
  pattern?: string | null;
  children: ReactNode;
}) {
  const { theme, fonts } = useDesign();
  // The student's page background (T-154) replaces the paper colour and picture; `label` is also the page kind of the layout.
  const lbg = useLayoutBg(label);
  const own = hasLayoutBg(lbg);
  return (
    <Sheet label={label} layer={<LayoutLayer kind={label} theme={theme} fonts={fonts} />}>
      <div
        className="mb-page"
        data-theme={theme || undefined}
        data-fonts={fonts || undefined}
        style={{
          backgroundColor: lbg.color ?? bg,
          backgroundImage: pattern && !own ? `url(${pattern})` : undefined,
        }}
      >
        {lbg.node}
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
      <Icon kind="heart" size={14} stroke="var(--mb-a1)" />
      <div>{y.graduation_year !== null && t("bookPages.classOf", { year: y.graduation_year })}</div>
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
  centered,
}: {
  id: string;
  fallback: string;
  fitKey: string;
  className: string;
  fitClass: string;
  style?: CSSProperties;
  centered?: boolean;
}) {
  const shown = useDesign().text(id, fallback);
  if (shown === fallback)
    return (
      <div className={className} style={style}>
        {fallback}
      </div>
    );
  return (
    <FitText fitKey={fitKey} className={`${className} ${fitClass}`} style={style}>
      {centered ? <span className="mb-centered">{shown}</span> : shown}
    </FitText>
  );
}

type Slot = { id: string; fallback: string };

/** A heading and its subtitle; a replaced one reports under `fitKey` and `fitKey:sub`. `subNode` is a subtitle that is not a text slot. */
function Title({
  head,
  sub,
  subNode,
  icon,
  color,
  fitKey,
}: {
  head: Slot;
  sub?: Slot;
  subNode?: ReactNode;
  icon: Kind;
  color: string;
  fitKey: string;
}) {
  return (
    <div>
      <div className="mb-title">
        <Own {...head} fitKey={fitKey} className="disp mb-title-text" fitClass="mb-title-fit" />
        <Icon kind={icon} size={34} stroke={color} />
      </div>
      {sub ? (
        <Own
          {...sub}
          fitKey={`${fitKey}:sub`}
          className="cute mb-title-sub"
          fitClass="mb-sub-fit"
        />
      ) : (
        subNode
      )}
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
  tail,
  children,
}: {
  fitKey: string;
  title: string;
  bg: string;
  icon: Kind;
  iconColor: string;
  grow?: number;
  side?: ReactNode;
  /** At the bottom right of the text box (the friend's signature, floated right; it comes first in the DOM, see .mb-sig-gap): the text wraps beside it, so it costs the area of the signature, not a full-width band. */
  tail?: ReactNode;
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
          {tail}
          {children}
        </FitText>
        {side}
      </div>
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
  const { image } = useDesign();
  const choice = image("cover.pattern") ?? "dots";
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
    <Page
      label="cover"
      bg="var(--mb-lav)"
      pattern={Object.hasOwn(PATTERNS, choice) ? PATTERNS[choice] : dots}
    >
      <div className="mb-spine" />
      {Array.from({ length: rings }, (_, i) => (
        <div key={i} className="mb-ring" style={{ top: 64 + i * 62 }} />
      ))}
      <div className="mb-cover-head">
        <Own
          id="cover.belongs"
          fallback={t("bookPages.memory.belongs")}
          fitKey="cover:belongs"
          className="cute"
          fitClass="mb-belongs-fit"
          style={{ fontSize: 32, color: SOFT }}
        />
        <FitText
          fitKey="cover:owner"
          className="disp mb-cover-owner"
          style={{ fontSize: 62 }}
          minScale={0.4}
        >
          {t("bookPages.memory.owner", { name: data.profile.full_name })}
        </FitText>
        <Editable layoutId="cover.title">
          <FitText
            fitKey="cover:title"
            className="disp mb-cover-title"
            style={{ fontSize: 100 }}
            minScale={0.35}
          >
            {y.title}
          </FitText>
        </Editable>
        {line && (
          <FitText fitKey="cover:class" className="mb-cover-class" style={{ fontSize: 14 }}>
            {line}
          </FitText>
        )}
      </div>
      <Editable layoutId="cover.photo">
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
      </Editable>
      <div
        className="mb-tape"
        style={{ left: 310, top: 394, width: 140, height: 28, transform: "rotate(-5deg)" }}
      />
      <Editable layoutId="cover.sticker">
        <div className="mb-sticker" style={{ left: 560, top: 680 }}>
          <Own
            id="cover.sticker"
            fallback={t("bookPages.memory.sticker")}
            fitKey="cover:sticker"
            className="cute mb-sticker-text"
            fitClass="mb-sticker-fit"
            centered
          />
        </div>
      </Editable>
      <Icon
        kind="heart"
        size={40}
        stroke="var(--mb-a1)"
        style={{ position: "absolute", left: 120, top: 440, transform: "rotate(-12deg)" }}
      />
      <Icon
        kind="sparkle"
        size={46}
        stroke="var(--mb-a2)"
        style={{ position: "absolute", left: 640, top: 420 }}
      />
      <Icon
        kind="star"
        size={30}
        stroke="var(--mb-a3)"
        style={{ position: "absolute", left: 600, top: 560, transform: "rotate(15deg)" }}
      />
      <Icon
        kind="sparkle"
        size={30}
        stroke="var(--mb-a2)"
        style={{ position: "absolute", left: 130, top: 800 }}
      />
      <Icon
        kind="heart"
        size={26}
        stroke="var(--mb-a2)"
        style={{ position: "absolute", left: 700, top: 900, transform: "rotate(14deg)" }}
      />
      <Icon
        kind="star"
        size={26}
        stroke="var(--mb-a3)"
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
    <Page label="profile" bg="var(--mb-paper)">
      <div className="mb-flow">
        <Editable layoutId="profile.heading">
          <Title
            fitKey="profile:heading"
            head={{ id: "profile.heading", fallback: t("bookPages.aboutMe") }}
            sub={{ id: "profile.subtitle", fallback: t("bookPages.memory.ownerOf") }}
            icon="sparkle"
            color="var(--mb-a2)"
          />
        </Editable>
        <div className="mb-head-row">
          <Editable layoutId="profile.photo">
            <Polaroid id={p.photo_media_id} className="mb-profile-photo" />
          </Editable>
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
            bg="var(--mb-peach)"
            icon="star"
            iconColor="var(--mb-a3)"
          >
            {p.hobbies}
          </Ruled>
          <Ruled
            fitKey="profile:future"
            title={t("bookPages.futurePlans")}
            bg="var(--mb-mint)"
            icon="heart"
            iconColor="var(--mb-a1)"
          >
            {p.future_plans}
          </Ruled>
        </div>
        <div className="mb-motto" style={{ flexGrow: rowWeight(p.quote.length) }}>
          <Icon kind="sparkle" size={22} stroke="var(--mb-a2)" />
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
        box("how_we_met", "var(--mb-peach)", "star", "var(--mb-a3)"),
        box("first_impression", "var(--mb-mint)", "sparkle", "var(--mb-a2)"),
      ],
    },
    {
      fields: ["best_memory"] as const,
      nodes: [box("best_memory", "var(--mb-sky)", "heart", "var(--mb-a1)")],
    },
    {
      fields: ["wish", "advice"] as const,
      nodes: [
        box("wish", "var(--mb-yellow)", "star", "var(--mb-a3)"),
        box("advice", "var(--mb-lav)", "sparkle", "var(--mb-a2)"),
      ],
    },
  ].filter((r) => r.nodes.some(Boolean));
  const extra = note.photo_ids.slice(1, 3);
  const signed = hasSignature(note.signature);
  return (
    <Page label="notes" bg="var(--mb-paper)">
      <div className="mb-flow">
        <Editable layoutId="notes.heading">
          <Title
            fitKey={`heading:${note.id}`}
            head={{ id: "notes.heading", fallback: t("bookPages.memory.friendTitle") }}
            icon="heart"
            color="var(--mb-a1)"
            subNode={
              <FitText
                fitKey={`sub:${note.id}`}
                className="cute mb-title-sub mb-title-line"
                style={{ fontSize: 26 }}
              >
                {data.profile.full_name &&
                  t("bookPages.memory.forYou", { name: data.profile.full_name })}
              </FitText>
            }
          />
        </Editable>
        <div className={`mb-head-row${signed ? " mb-signed" : ""}`}>
          {note.photo_ids[0] ? (
            <Polaroid id={note.photo_ids[0]} className="mb-friend-photo" />
          ) : (
            <div className="mb-stick">
              <Icon kind="heart" size={28} stroke="var(--mb-a1)" />
              <div className="cute mb-stick-text">{t("bookPages.memory.stickPhoto")}</div>
            </div>
          )}
          <div className="mb-fields" style={{ height: signed ? 190 : 220 }}>
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
                bg="var(--mb-lav)"
                icon="heart"
                iconColor="var(--mb-a1)"
                tail={
                  signed && (
                    <>
                      <span className="mb-sig-gap" aria-hidden="true" />
                      <BookSignature
                        value={note.signature}
                        label={t("note.signature.of", { name: a.name ?? "" })}
                        className="mb-sig"
                      />
                    </>
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
    <Page label="back" bg="var(--mb-paper)">
      <div className="mb-flow">
        <Editable layoutId="back.heading">
          <Title
            fitKey="back:heading"
            head={{ id: "back.heading", fallback: t("bookPages.memory.touchTitle") }}
            sub={{ id: "back.subtitle", fallback: t("bookPages.memory.touchSub") }}
            icon="heart"
            color="var(--mb-a1)"
          />
        </Editable>
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
        <Editable layoutId="back.pact">
          <div className="mb-pact">
            <div className="mb-pact-text">
              <div className="mb-pact-head">
                <Icon kind="star" size={22} stroke="var(--mb-a3)" />
                <Own
                  id="back.pact"
                  fallback={t("bookPages.memory.pact")}
                  fitKey="back:pact"
                  className="disp"
                  fitClass="mb-pact-fit"
                  style={{ fontSize: 22, fontWeight: 600 }}
                />
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
        </Editable>
      </div>
      <Icon
        kind="sparkle"
        size={30}
        stroke="var(--mb-a2)"
        style={{ position: "absolute", left: 700, top: 66 }}
      />
      <div className="mb-foot">
        <Footer data={data} />
      </div>
    </Page>
  );
}

/**
 * What the layout layer needs of this template (T-154). `colors` and `fonts` are the CSS behind the manifest's `design.layout` tokens; `natural` lists the
 * elements the student may move (the manifest's `editable` ids; a test pins them) with the place each has on the A5 page, in design px, which an override
 * that omits a field starts from. Sizes come from the template's CSS; the tops of the elements in the page's flow are approximate (the editor writes every field from the element's real box, so an omitted field only matters for a hand-made document). `flow` elements leave a blank of that size when moved.
 */
const LAYOUT: TemplateLayout = {
  colors: {
    ink: "var(--mb-ink)",
    soft: "var(--mb-soft)",
    paper: "var(--mb-paper)",
    white: "var(--mb-white)",
    accent: "var(--mb-a1)",
  },
  fonts: { display: "var(--mb-disp)", body: "var(--mb-body)" },
  natural: {
    "cover.photo": { left: 190, top: 410, w: 370, h: 400, r: -3 },
    "cover.sticker": { left: 560, top: 680, w: 176, h: 176, r: 10 },
    "cover.title": { left: 96, top: 166, w: 672, h: 120, flow: true },
    "profile.heading": { left: 48, top: 52, w: 720, h: 84, flow: true },
    "profile.photo": { left: 48, top: 154, w: 240, h: 290, r: -2, flow: true },
    "notes.heading": { left: 48, top: 52, w: 596, h: 84, flow: true },
    "back.heading": { left: 48, top: 52, w: 720, h: 84, flow: true },
    "back.pact": { left: 48, top: 850, w: 720, h: 105, flow: true },
  },
};

export const memoryBook: Template = {
  id: "memory-book",
  notesPerPage: 1,
  design: { themes: ["lavender", "peach", "mint"], fonts: ["playful", "classic"] },
  swatch: {
    root: "mb-page",
    chips: ["--mb-a1", "--mb-a2", "--mb-a3", "--mb-lav", "--mb-mint"],
    serif: "disp",
  },
  // the locale key of each text's own wording (the customise screen shows it as the placeholder); design.test.ts pins the ids to the manifest
  textDefaults: {
    "cover.belongs": "bookPages.memory.belongs",
    "cover.sticker": "bookPages.memory.sticker",
    "profile.heading": "bookPages.aboutMe",
    "profile.subtitle": "bookPages.memory.ownerOf",
    "notes.heading": "bookPages.memory.friendTitle",
    "back.heading": "bookPages.memory.touchTitle",
    "back.subtitle": "bookPages.memory.touchSub",
    "back.pact": "bookPages.memory.pact",
  },
  layout: LAYOUT,
  Cover,
  Profile,
  Notes,
  Back,
};
