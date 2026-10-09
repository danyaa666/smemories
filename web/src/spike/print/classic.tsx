// Design temp2 "Navy classic" as data-bound components. Source: design/canvas/temp2.html (nested pages
// Yearbook cover, Welcome letter, Autographs). The autograph pages show the friends' notes and need pagination.
import { useLayoutEffect, useRef, useState } from "react";
import { STRINGS, type Book, type Friend, type Lang } from "./data";
import { designHeight, FitText, Photo, Sheet, useSize } from "./sheet";

const GOLD = "#C9A86A";
const BROWN = "#7A5C2B";

function Ornament({ w = 220, color = BROWN }: { w?: number; color?: string }) {
  const m = w / 2;
  return (
    <svg width={w} height={14} viewBox={`0 0 ${w} 14`} aria-hidden="true">
      <path d={`M0 7h${m - 14}M${m + 14} 7h${m - 14}`} stroke={color} strokeWidth={1} />
      <path d={`M${m} 1l6 6-6 6-6-6z`} fill="none" stroke={color} strokeWidth={1.2} />
      <circle cx={m} cy={7} r={1.6} fill={color} />
    </svg>
  );
}

export function Cover({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  return (
    <Sheet label="classic-cover" className="cl" style={{ background: "#1B2A4A" }}>
      <div style={{ position: "absolute", inset: 36, border: `1px solid ${GOLD}` }} />
      <div style={{ position: "absolute", inset: 44, border: `1px solid ${GOLD}` }} />
      <div
        style={{
          position: "absolute",
          left: 80,
          right: 80,
          top: 150,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 18,
          textAlign: "center",
          color: "#F7F3EA",
        }}
      >
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
        <FitText
          className="caps"
          style={{
            fontSize: 16,
            letterSpacing: ".36em",
            width: "100%",
            height: 24,
            whiteSpace: "nowrap",
          }}
        >
          {book.school}
        </FitText>
        <Ornament w={260} color={GOLD} />
        <div
          className="serif"
          style={{
            fontSize: 64,
            fontStyle: "italic",
            fontWeight: 500,
            lineHeight: 1,
            marginTop: 18,
          }}
        >
          {lang === "vi" ? "Khóa" : "Class of"}
        </div>
        <div
          className="serif"
          style={{
            fontSize: 230,
            fontWeight: 600,
            lineHeight: 0.82,
            color: GOLD,
            letterSpacing: "-.01em",
          }}
        >
          {book.year}
        </div>
        <div style={{ marginTop: 26 }}>
          <Ornament w={260} color={GOLD} />
        </div>
        <div className="caps" style={{ fontSize: 14 }}>
          {t.gradYearbook}
        </div>
      </div>
      <div
        style={{
          position: "absolute",
          left: 80,
          right: 80,
          bottom: 92,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 6,
          color: "#F7F3EA",
          textAlign: "center",
        }}
      >
        <div className="serif" style={{ fontSize: 24, fontStyle: "italic" }}>
          {book.quote}
        </div>
        <div className="caps" style={{ fontSize: 11, color: GOLD }}>
          Volume 1 · {book.city}
        </div>
      </div>
    </Sheet>
  );
}

export function Letter({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  return (
    <Sheet label="classic-letter" className="cl" style={{ background: "#F7F3EA" }}>
      <div
        style={{
          position: "absolute",
          left: 56,
          right: 56,
          top: 72,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 10,
          textAlign: "center",
        }}
      >
        <div className="caps" style={{ fontSize: 12, color: BROWN }}>
          {t.message}
        </div>
        <div className="serif" style={{ fontSize: 68, fontWeight: 600, lineHeight: 1 }}>
          {t.letterTitle} {book.year},
        </div>
        <Ornament />
      </div>
      <div
        style={{
          position: "absolute",
          left: 72,
          right: 72,
          top: 270,
          bottom: 120,
          display: "flex",
          gap: 48,
          alignItems: "flex-start",
        }}
      >
        <FitText
          className="serif"
          style={{ flexGrow: 1, height: "100%", fontSize: 22, lineHeight: 1.55 }}
        >
          {book.letter.map((p, i) => (
            <p key={i} style={{ margin: "0 0 18px" }}>
              {p}
            </p>
          ))}
          <div style={{ fontSize: 40, fontStyle: "italic", color: BROWN }}>Trần Minh Hạnh</div>
          <div className="caps" style={{ fontSize: 12 }}>
            Trần Minh Hạnh · {lang === "vi" ? "Hiệu trưởng" : "Principal"}
          </div>
        </FitText>
        <div
          style={{
            width: 220,
            flexShrink: 0,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: 14,
          }}
        >
          <Photo
            src={book.profilePhoto}
            style={{
              width: 200,
              height: 260,
              borderRadius: "50%",
              outline: `1px solid ${GOLD}`,
              outlineOffset: 8,
            }}
          />
          <div
            className="serif"
            style={{ fontSize: 17, fontStyle: "italic", textAlign: "center", marginTop: 8 }}
          >
            Trần Minh Hạnh, {lang === "vi" ? "Hiệu trưởng" : "Principal"}
          </div>
        </div>
      </div>
      <div
        className="caps"
        style={{
          position: "absolute",
          left: 56,
          right: 56,
          bottom: 36,
          borderTop: `1px solid ${GOLD}`,
          paddingTop: 12,
          display: "flex",
          justifyContent: "space-between",
          fontSize: 11,
        }}
      >
        <span>Class of {book.year}</span>
        <span>2</span>
      </div>
    </Sheet>
  );
}

const COL_W = 304; // (816 - 2 * 80 - 48) / 2

function Note({ f }: { f: Friend }) {
  return (
    <div className="cl-note">
      <div className="serif" style={{ fontSize: 17, lineHeight: 1.35 }}>
        {f.answers.wish}
      </div>
      <div className="caps" style={{ fontSize: 10, color: BROWN, marginTop: 4 }}>
        — {f.name}
      </div>
    </div>
  );
}

/**
 * Pagination of the friends' notes: render every note once in a hidden column of the real width, measure it, then
 * fill two columns per page greedily. A note taller than a whole column is shrunk to fit (see FitText), it is not split.
 */
export function Autographs({ book, lang }: { book: Book; lang: Lang }) {
  const t = STRINGS[lang];
  const size = useSize();
  const colH = designHeight(size) - 250 - 230;
  const probe = useRef<HTMLDivElement>(null);
  const [pages, setPages] = useState<Friend[][][] | null>(null); // page -> column -> notes
  useLayoutEffect(() => {
    let live = true;
    void document.fonts.ready.then(() => {
      if (!live || !probe.current) return;
      const hs = Array.from(probe.current.children, (c) =>
        Math.min((c as HTMLElement).offsetHeight, colH),
      );
      const out: Friend[][][] = [[[], []]];
      let col = 0;
      let used = 0;
      book.friends.forEach((f, i) => {
        const h = (hs[i] ?? 0) + 16;
        if (used + h > colH) {
          used = 0;
          if (col === 1) {
            out.push([[], []]);
            col = 0;
          } else col = 1;
        }
        (out[out.length - 1] as Friend[][])[col]?.push(f);
        used += h;
      });
      setPages(out);
    });
    return () => {
      live = false;
    };
  }, [book.friends, colH]);
  return (
    <>
      <div ref={probe} className="cl-probe" style={{ width: COL_W }} aria-hidden="true">
        {book.friends.map((f) => (
          <Note key={f.id} f={f} />
        ))}
      </div>
      {(pages ?? [[[], []]]).map((cols, p) => (
        <Sheet
          key={p}
          label={`classic-autographs-${p + 1}`}
          className="cl"
          style={{ background: "#F7F3EA" }}
        >
          <div data-pending={pages ? undefined : "1"} />
          <div style={{ position: "absolute", inset: 28, border: `1px solid ${GOLD}` }} />
          <div style={{ position: "absolute", inset: 34, border: `1px solid ${GOLD}` }} />
          <div
            style={{
              position: "absolute",
              left: 56,
              right: 56,
              top: 76,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 10,
              textAlign: "center",
            }}
          >
            <div className="caps" style={{ fontSize: 12, color: BROWN }}>
              {t.words}
            </div>
            <div className="serif" style={{ fontSize: 68, fontWeight: 600, lineHeight: 1 }}>
              {t.auto}
            </div>
            <Ornament />
          </div>
          <div
            style={{
              position: "absolute",
              left: 80,
              right: 80,
              top: 250,
              height: colH,
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: 48,
            }}
          >
            {cols.map((notes, c) => (
              <div key={c} className="cl-lines">
                {notes.map((f) => (
                  <FitText key={f.id} style={{ maxHeight: colH, overflow: "hidden" }}>
                    <Note f={f} />
                  </FitText>
                ))}
              </div>
            ))}
          </div>
          <div
            style={{
              position: "absolute",
              left: 80,
              right: 80,
              bottom: 84,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 8,
              textAlign: "center",
            }}
          >
            <Ornament />
            <div className="serif" style={{ fontSize: 38, fontStyle: "italic", fontWeight: 500 }}>
              {t.farewell} {book.year}
            </div>
            <div className="caps" style={{ fontSize: 11, color: BROWN }}>
              {book.school}
            </div>
          </div>
        </Sheet>
      ))}
    </>
  );
}

export function ClassicBook({ book, lang }: { book: Book; lang: Lang }) {
  return (
    <>
      <Cover book={book} lang={lang} />
      <Letter book={book} lang={lang} />
      <Autographs book={book} lang={lang} />
    </>
  );
}
