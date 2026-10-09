// Design temp1 "Memory Book" as data-bound components. Source: design/canvas/temp1.html (nested pages
// Cover, All about me, Friend page, Let's stay in touch). Differences from the canvas HTML are listed in ADR 0003.
import type { ReactNode } from "react";
import { STRINGS, type Book, type Friend, type Lang } from "./data";
import { FitText, Icon, Photo, Sheet } from "./sheet";

const INK = "#3B2F4A";
const SOFT = "#5E5170";

function Footer({ book }: { book: Book }) {
  return (
    <div className="mb-footer">
      <div>{book.owner.name}’s Memory Book</div>
      <Icon kind="heart" size={14} stroke="#D9668C" />
      <div>Class of {book.year}</div>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="mb-field">
      <div className="mb-label">{label}</div>
      <FitText className="mb-value">{value}</FitText>
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
  sub: string;
  icon: "heart" | "sparkle";
  color: string;
}) {
  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <div className="disp" style={{ fontSize: 52, fontWeight: 600, lineHeight: 1.05 }}>
          {children}
        </div>
        <Icon kind={icon} size={34} stroke={color} />
      </div>
      <div className="cute" style={{ fontSize: 26, color: SOFT, lineHeight: 1.1 }}>
        {sub}
      </div>
    </div>
  );
}

function Ruled({
  title,
  bg,
  icon,
  iconColor,
  lines,
  children,
  side,
}: {
  title: string;
  bg: string;
  icon: "heart" | "sparkle" | "star";
  iconColor: string;
  lines: number;
  children: ReactNode;
  side?: ReactNode;
}) {
  return (
    <div className="mb-ruled" style={{ background: bg }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <Icon kind={icon} size={20} stroke={iconColor} />
        <div className="disp" style={{ fontSize: 19, fontWeight: 600 }}>
          {title}
        </div>
      </div>
      <div style={{ display: "flex", gap: 14 }}>
        <FitText className="mb-lines" style={{ height: lines * 34, flexGrow: 1 }}>
          {children}
        </FitText>
        {side}
      </div>
    </div>
  );
}

export function Cover({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  return (
    <Sheet label="memory-cover" className="sp-dots" style={{ backgroundColor: "#E6DEFA" }}>
      <div
        style={{
          position: "absolute",
          left: 0,
          top: 0,
          bottom: 0,
          width: 62,
          background: "rgba(255,255,255,.55)",
        }}
      />
      {Array.from({ length: 15 }, (_, i) => (
        <div
          key={i}
          style={{
            position: "absolute",
            left: 18,
            top: 64 + i * 62,
            width: 26,
            height: 26,
            borderRadius: "50%",
            border: `3px solid ${SOFT}`,
            background: "#FFFBF6",
          }}
        />
      ))}
      <div
        style={{
          position: "absolute",
          left: 96,
          right: 48,
          top: 92,
          display: "flex",
          flexDirection: "column",
          gap: 4,
        }}
      >
        <div className="cute" style={{ fontSize: 32, color: SOFT }}>
          {t.belongs}
        </div>
        <FitText
          className="disp"
          style={{
            fontSize: 62,
            fontWeight: 600,
            lineHeight: "70px",
            height: 70,
            whiteSpace: "nowrap",
          }}
        >
          {book.owner.name}’s
        </FitText>
        <div
          className="disp"
          style={{ fontSize: 100, fontWeight: 600, lineHeight: 1, letterSpacing: "-.01em" }}
        >
          {t.book}
        </div>
        <FitText
          style={{
            fontSize: 14,
            fontWeight: 700,
            letterSpacing: ".14em",
            textTransform: "uppercase",
            marginTop: 10,
            height: 20,
            whiteSpace: "nowrap",
          }}
        >
          Class of {book.year} · {book.owner.className} · {book.school}
        </FitText>
      </div>
      <div style={{ position: "absolute", left: 190, top: 410 }}>
        <div
          className="mb-polaroid"
          style={{ width: 370, padding: 14, borderRadius: 20, transform: "rotate(-3deg)" }}
        >
          <Photo src={book.coverPhoto} style={{ width: "100%", height: 372, borderRadius: 12 }} />
        </div>
      </div>
      <div
        style={{
          position: "absolute",
          left: 310,
          top: 394,
          width: 140,
          height: 28,
          transform: "rotate(-5deg)",
          opacity: 0.9,
          backgroundImage: "repeating-linear-gradient(45deg, #FCD5E2 0 8px, #FFFFFF 8px 16px)",
        }}
      />
      <div className="mb-sticker" style={{ left: 560, top: 680 }}>
        <div
          className="cute"
          style={{ fontSize: 32, fontWeight: 700, lineHeight: 1, whiteSpace: "pre-line" }}
        >
          {t.write}
        </div>
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
      <div
        style={{
          position: "absolute",
          left: 96,
          right: 48,
          bottom: 64,
          display: "flex",
          justifyContent: "space-between",
          alignItems: "flex-end",
        }}
      >
        <div className="cute" style={{ fontSize: 28, lineHeight: 1.1 }}>
          {book.quote}
        </div>
        <div style={{ fontSize: 14, fontWeight: 700, letterSpacing: ".12em" }}>
          {book.year - 3} – {book.year}
        </div>
      </div>
    </Sheet>
  );
}

export function Profile({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  const o = book.owner;
  return (
    <Sheet label="memory-profile" className="sp-dots" style={{ backgroundColor: "#FFFBF6" }}>
      <div
        style={{
          position: "absolute",
          left: 48,
          right: 48,
          top: 52,
          display: "flex",
          flexDirection: "column",
          gap: 18,
        }}
      >
        <Title sub={o.name} icon="sparkle" color="#8E6CCB">
          {t.mine}
        </Title>
        <div style={{ display: "flex", gap: 32, alignItems: "center" }}>
          <div
            className="mb-polaroid"
            style={{ width: 240, padding: 10, borderRadius: 20, transform: "rotate(-2deg)" }}
          >
            <Photo
              src={book.profilePhoto}
              style={{ width: "100%", height: 270, borderRadius: 12 }}
            />
          </div>
          <div
            style={{
              flexGrow: 1,
              height: 280,
              display: "flex",
              flexDirection: "column",
              justifyContent: "space-between",
            }}
          >
            <Field label={t.fullName} value={o.name} />
            <Field label={t.nick} value={o.nickname} />
            <Field label={t.birthday} value={o.birthday} />
            <Field label={t.zodiac} value={o.zodiac} />
            <Field label={t.cls} value={o.className} />
            <Field label={t.hobbies} value={o.hobbies} />
            <Field label={t.best} value={o.bestFriends} />
          </div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, minmax(0, 1fr))", gap: 14 }}>
          {(
            [
              [t.food, o.food, "#FFDCCB"],
              [t.song, o.song, "#E6DEFA"],
              [t.movie, o.movie, "#D2EEDF"],
              [t.subject, o.subject, "#D3E8F8"],
            ] as const
          ).map(([l, v, bg]) => (
            <div
              key={l}
              style={{
                background: bg,
                borderRadius: 16,
                padding: "12px 14px",
                display: "flex",
                flexDirection: "column",
                gap: 4,
              }}
            >
              <div
                style={{
                  fontSize: 13,
                  fontWeight: 700,
                  letterSpacing: ".08em",
                  textTransform: "uppercase",
                }}
              >
                {l}
              </div>
              <FitText className="mb-value" style={{ height: 34, lineHeight: "32px" }}>
                {v}
              </FitText>
            </div>
          ))}
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 18 }}>
          <Ruled title={t.dream} bg="#FFDCCB" icon="star" iconColor="#E09A2D" lines={5}>
            {o.dream}
          </Ruled>
          <Ruled title={t.miss} bg="#D2EEDF" icon="heart" iconColor="#D9668C" lines={5}>
            {o.miss}
          </Ruled>
        </div>
        <div
          style={{
            background: "#FFF1B5",
            borderRadius: 18,
            padding: "10px 18px",
            display: "flex",
            alignItems: "flex-end",
            gap: 12,
          }}
        >
          <Icon kind="sparkle" size={22} stroke="#8E6CCB" />
          <div
            className="disp"
            style={{ fontSize: 19, fontWeight: 600, whiteSpace: "nowrap", paddingBottom: 4 }}
          >
            {t.motto}
          </div>
          <FitText className="mb-value" style={{ flexGrow: 1, height: 36, lineHeight: "34px" }}>
            {o.motto}
          </FitText>
        </div>
      </div>
      <div style={{ position: "absolute", left: 48, right: 48, bottom: 40 }}>
        <Footer book={book} />
      </div>
    </Sheet>
  );
}

export function FriendPage({ book, friend, lang }: { book: Book; friend: Friend; lang: Lang }) {
  const t = STRINGS[lang];
  const a = friend.answers;
  return (
    <Sheet
      label={`memory-friend-${friend.id}`}
      className="sp-dots"
      style={{ backgroundColor: "#FFFBF6" }}
    >
      <div
        style={{
          position: "absolute",
          left: 48,
          right: 48,
          top: 52,
          display: "flex",
          flexDirection: "column",
          gap: 18,
        }}
      >
        <Title sub={friend.nickname} icon="heart" color="#D9668C">
          {t.friend}
        </Title>
        <div style={{ display: "flex", gap: 28, alignItems: "stretch" }}>
          <div
            className="mb-polaroid"
            style={{
              width: 190,
              height: 220,
              flexShrink: 0,
              padding: 8,
              borderRadius: 18,
              transform: "rotate(-2deg)",
            }}
          >
            <Photo
              src={friend.photos[0]}
              style={{ width: "100%", height: "100%", borderRadius: 12 }}
            />
          </div>
          <div
            style={{
              flexGrow: 1,
              height: 220,
              display: "flex",
              flexDirection: "column",
              justifyContent: "space-between",
            }}
          >
            <Field label={t.name} value={friend.name} />
            <Field label={t.nick} value={friend.nickname} />
            <Field label={t.birthday} value={friend.birthday} />
            <Field label={t.phone} value={friend.phone} />
            <Field label={t.social} value={friend.social} />
            <Field label={t.email} value={friend.email} />
          </div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 18 }}>
          <Ruled title={t.howMet} bg="#FFDCCB" icon="star" iconColor="#E09A2D" lines={3}>
            {a.how_we_met}
          </Ruled>
          <Ruled title={t.impression} bg="#D2EEDF" icon="sparkle" iconColor="#8E6CCB" lines={3}>
            {a.first_impression}
          </Ruled>
        </div>
        <Ruled
          title={t.memory}
          bg="#D3E8F8"
          icon="heart"
          iconColor="#D9668C"
          lines={4}
          side={
            friend.photos[1] ? (
              <div
                className="mb-polaroid"
                style={{
                  width: 150,
                  height: 120,
                  padding: 6,
                  borderRadius: 12,
                  transform: "rotate(3deg)",
                  flexShrink: 0,
                }}
              >
                <Photo
                  src={friend.photos[1]}
                  style={{ width: "100%", height: "100%", borderRadius: 8 }}
                />
              </div>
            ) : null
          }
        >
          {a.best_memory}
        </Ruled>
        <div style={{ display: "flex", gap: 18 }}>
          <div style={{ flexGrow: 1 }}>
            <Ruled title={t.wishFor} bg="#FFF1B5" icon="star" iconColor="#E09A2D" lines={3}>
              {a.wish}
            </Ruled>
          </div>
          <div
            style={{ width: 262, flexShrink: 0, display: "flex", flexDirection: "column", gap: 6 }}
          >
            <div
              style={{
                background: "#E6DEFA",
                borderRadius: 18,
                padding: "12px 16px",
                display: "flex",
                flexDirection: "column",
                gap: 6,
              }}
            >
              <div className="disp" style={{ fontSize: 17, fontWeight: 600 }}>
                {t.rate}
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                {[1, 2, 3, 4, 5].map((n) => (
                  <Icon
                    key={n}
                    kind="heart"
                    size={30}
                    stroke={n <= friend.rating ? "#D9668C" : SOFT}
                    fill={n <= friend.rating ? "#F4A6C0" : "none"}
                  />
                ))}
              </div>
            </div>
            <Field label={t.signed} value={friend.nickname} />
          </div>
        </div>
      </div>
      <div
        style={{
          position: "absolute",
          right: 48,
          top: 56,
          padding: "8px 14px",
          background: "#FFF1B5",
          borderRadius: 12,
          transform: "rotate(4deg)",
          fontSize: 14,
          fontWeight: 700,
          color: INK,
        }}
      >
        No. {friend.id}
      </div>
      <div style={{ position: "absolute", left: 48, right: 48, bottom: 40 }}>
        <Footer book={book} />
      </div>
    </Sheet>
  );
}

export function Contacts({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  const head = [
    ["#", 36],
    [t.name, 210],
    [t.phone, 150],
    [t.social, 0],
    [t.birthday, 110],
  ] as const;
  return (
    <Sheet label="memory-contacts" className="sp-dots" style={{ backgroundColor: "#FFFBF6" }}>
      <div
        style={{
          position: "absolute",
          left: 48,
          right: 48,
          top: 52,
          display: "flex",
          flexDirection: "column",
          gap: 18,
        }}
      >
        <Title sub={t.sub} icon="heart" color="#D9668C">
          {t.touch}
        </Title>
        {/* The canvas uses <sc-raw-table> custom elements here; a real table (or CSS grid) is needed in JSX. */}
        <table className="mb-table">
          <thead>
            <tr>
              {head.map(([h, w]) => (
                <th key={h} style={w ? { width: w } : undefined}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {book.friends.map((f, i) => (
              <tr key={f.id}>
                <td style={{ textAlign: "center", fontWeight: 700 }}>{i + 1}</td>
                <td>{f.name}</td>
                <td>{f.phone}</td>
                <td>{f.social}</td>
                <td>{f.birthday}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div style={{ position: "absolute", left: 48, right: 48, bottom: 40 }}>
        <Footer book={book} />
      </div>
    </Sheet>
  );
}

export function Back({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  return (
    <Sheet label="memory-back" className="sp-dots" style={{ backgroundColor: "#E6DEFA" }}>
      <div style={{ position: "absolute", left: 0, right: 0, top: 380, textAlign: "center" }}>
        <div className="disp" style={{ fontSize: 64, fontWeight: 600 }}>
          {book.owner.nickname}
        </div>
        <div className="cute" style={{ fontSize: 32, color: SOFT }}>
          {t.thanks}
        </div>
      </div>
      <Icon
        kind="heart"
        size={60}
        stroke="#D9668C"
        fill="#F4A6C0"
        style={{ position: "absolute", left: 378, top: 560 }}
      />
      <Icon
        kind="sparkle"
        size={40}
        stroke="#8E6CCB"
        style={{ position: "absolute", left: 220, top: 300 }}
      />
      <Icon
        kind="star"
        size={36}
        stroke="#E09A2D"
        style={{ position: "absolute", left: 560, top: 640 }}
      />
    </Sheet>
  );
}

export function MemoryBook({ book, lang }: { book: Book; lang: Lang }) {
  return (
    <>
      <Cover book={book} lang={lang} />
      <Profile book={book} lang={lang} />
      {book.friends.map((f) => (
        <FriendPage key={f.id} book={book} friend={f} lang={lang} />
      ))}
      <Contacts book={book} lang={lang} />
      <Back book={book} lang={lang} />
    </>
  );
}
