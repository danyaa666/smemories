import type { TFunction } from "i18next";
import { createContext, useContext, useState, type ReactNode } from "react";
import { mediaUrl } from "../api/yearbooks";
import { QueuedImg } from "../components/QueuedImg";
import { designHeight, designZoom, DESIGN_W, type PageSize } from "./sizes";

/** What every page of the book needs: its size, its own language (the book's, not the app's) and whether images load eagerly. */
export type BookEnv = { size: PageSize; t: TFunction; eager: boolean };
const Env = createContext<BookEnv | null>(null);
export const BookEnvProvider = Env.Provider;
export function useBookEnv(): BookEnv {
  const e = useContext(Env);
  if (!e) throw new Error("BookEnvProvider is missing");
  return e;
}

/**
 * One printed page: a box in mm (CSS variables --pw and --ph on the stage) that breaks after itself, with the design,
 * drawn in a box DESIGN_W px wide, zoomed to the paper. On screen the stage scales the box with a transform (no layout change,
 * so text fitting is the same on screen and on paper); in print the transform is gone (book.css).
 */
export function Sheet({ label, children }: { label: string; children: ReactNode }) {
  const { size } = useBookEnv();
  return (
    <div className="bk-frame">
      <section className="bk-sheet" data-page={label}>
        <div
          className="bk-inner"
          style={{ width: DESIGN_W, height: designHeight(size), zoom: designZoom(size) }}
        >
          {children}
        </div>
      </section>
    </div>
  );
}

/**
 * A photo in the print size (T-057). Lazy on screen; the book preloads every photo before printing is allowed, and
 * `eager` (set while the print dialog opens) makes the browser fetch whatever is left. A photo that fails shows an empty frame.
 */
export function Photo({ id, className }: { id: string | null | undefined; className?: string }) {
  const { eager } = useBookEnv();
  const [failed, setFailed] = useState(false);
  if (!id || failed) return <div className={`bk-ph ${className ?? ""}`} />;
  return (
    <QueuedImg
      className={`bk-photo ${className ?? ""}`}
      src={mediaUrl(id, "print")}
      alt=""
      eager={eager}
      decoding={eager ? "sync" : "async"}
      onError={() => setFailed(true)}
    />
  );
}
