import {
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import { useTranslation } from "react-i18next";
import type { NoteItem } from "../api/notes";
import { mediaUrl } from "../api/yearbooks";
import { Badge } from "./ui/Badge";
import { Button } from "./ui/Button";

/** Field ids of the catalogue (docs/note-fields.md) that have a label in the locale files; any other id is shown as it is. */
const KNOWN_FIELDS = [
  "name",
  "nickname",
  "relationship",
  "message",
  "how_we_met",
  "first_impression",
  "best_memory",
  "wish",
  "advice",
];
/** Answers the card header and body already show. */
const SHOWN_ABOVE = new Set(["name", "relationship", "message"]);

export const noteName = (n: NoteItem, anonymous: string) => n.answers.name?.trim() || anonymous;

/** "2 hours ago" in the UI language. */
export function ago(ms: number, lang: string, now = Date.now()): string {
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" });
  const s = (ms - now) / 1000;
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [
    ["minute", 60],
    ["hour", 3600],
    ["day", 86400],
    ["week", 604800],
  ];
  if (Math.abs(s) < 60) return rtf.format(0, "second");
  for (let i = steps.length - 1; i >= 0; i--) {
    const [unit, size] = steps[i]!;
    if (Math.abs(s) >= size) {
      const v = Math.trunc(s / size);
      // beyond a month an absolute date is clearer than "7 weeks ago"
      return Math.abs(s) >= 4 * 604800
        ? new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(ms)
        : rtf.format(v, unit);
    }
  }
  return rtf.format(0, "second");
}

export type DragState = { id: string; over: string } | null;

type Props = {
  note: NoteItem;
  lang: string;
  selected: boolean;
  onSelect: () => void;
  onStatus: (status: "approved" | "hidden") => void;
  onDelete: () => void;
  /** Set only in the approved tab: the note's place and the move handlers. */
  order?: {
    position: number;
    total: number;
    dragging: DragState;
    onMove: (to: number) => void;
    onDragStart: () => void;
    onDragOver: (overId: string) => void;
    onDragEnd: (commit: boolean) => void;
    /** Index of a note id in the list, for the drop target. */
    indexOf: (id: string) => number;
  };
};

/** Props shared by the grip of a card and of a compact order row. */
export function gripProps(
  id: string,
  order: Order,
  label: string,
): ButtonHTMLAttributes<HTMLButtonElement> {
  const idx = order.position - 1;
  return {
    "aria-label": label,
    onPointerDown: (e: PointerEvent<HTMLButtonElement>) => {
      if (e.button > 0) return; // the primary button, a finger or a pen
      e.currentTarget.setPointerCapture?.(e.pointerId);
      order.onDragStart();
    },
    onPointerMove: (e: PointerEvent<HTMLButtonElement>) => {
      if (order.dragging?.id !== id) return;
      const el = document
        .elementFromPoint(e.clientX, e.clientY)
        ?.closest<HTMLElement>("[data-note-id]");
      if (el?.dataset.noteId) order.onDragOver(el.dataset.noteId);
    },
    onPointerUp: () => order.onDragEnd(true),
    onPointerCancel: () => order.onDragEnd(false),
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key === "ArrowUp") {
        e.preventDefault();
        order.onMove(idx - 1);
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        order.onMove(idx + 1);
      }
    },
  };
}

type Order = NonNullable<Props["order"]>;

/** Classes for a row in a drag: dimmed while it is the one dragged, a drop line on the row it would land on. */
function dropClass(id: string, order?: Order): string {
  const d = order?.dragging;
  if (!order || !d) return "";
  if (d.id === id) return "is-dragging";
  if (d.over !== id) return "";
  // moving down puts the note after the target, moving up before it
  return order.indexOf(d.id) < order.position - 1 ? "drop-below" : "drop-above";
}

export function NoteCard({ note, lang, selected, onSelect, onStatus, onDelete, order }: Props) {
  const { t } = useTranslation();
  const name = noteName(note, t("inbox.anonymous"));
  const menuId = useId();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLLIElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: globalThis.KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  const meta = [note.answers.relationship?.trim(), ago(note.created_at, lang), note.link_label]
    .filter(Boolean)
    .join(" · ");
  const extra = Object.entries(note.answers).filter(([k, v]) => !SHOWN_ABOVE.has(k) && v?.trim());
  const cls = ["inbox-card", selected && "is-selected", dropClass(note.id, order)]
    .filter(Boolean)
    .join(" ");

  return (
    <li ref={root} className={cls} data-note-id={note.id}>
      <div className="inbox-card-head">
        <input
          type="checkbox"
          className="inbox-check"
          checked={selected}
          onChange={onSelect}
          aria-label={t("inbox.select", { name })}
        />
        {order && (
          <button
            type="button"
            className="inbox-grip"
            {...gripProps(note.id, order, t("inbox.grip", { name }))}
          >
            <span aria-hidden="true">⠿</span>
          </button>
        )}
        <span className="avatar" aria-hidden="true">
          {Array.from(name)[0]?.toUpperCase()}
        </span>
        <div className="inbox-card-who">
          <strong>{name}</strong>
          <small>{meta}</small>
        </div>
        {order && (
          <span className="inbox-pos">
            {t("inbox.position", { n: order.position, total: order.total })}
          </span>
        )}
        <Badge variant={note.status}>{t(`inbox.status.${note.status}`)}</Badge>
      </div>
      {note.answers.message && <p className="inbox-msg">{note.answers.message}</p>}
      {extra.length > 0 && (
        <dl className="inbox-extra">
          {extra.map(([k, v]) => (
            <div key={k}>
              <dt>{KNOWN_FIELDS.includes(k) ? t(`inbox.fields.${k}`) : k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
      )}
      {note.photos.length > 0 && (
        <ul className="inbox-photos">
          {note.photos.map((p, i) => (
            <li key={p.id}>
              <a
                href={mediaUrl(p.id, "display")}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={t("inbox.openPhoto", { n: i + 1, name })}
              >
                <img src={mediaUrl(p.id, "thumb")} alt="" loading="lazy" />
              </a>
            </li>
          ))}
        </ul>
      )}
      <div className="inbox-actions">
        {note.status !== "approved" && (
          <Button variant="secondary" onClick={() => onStatus("approved")}>
            {t("inbox.approve")} <span className="visually-hidden">{name}</span>
          </Button>
        )}
        {note.status !== "hidden" && (
          <Button variant="secondary" onClick={() => onStatus("hidden")}>
            {t("inbox.hide")} <span className="visually-hidden">{name}</span>
          </Button>
        )}
        <div className="inbox-more">
          <Button
            variant="ghost"
            aria-haspopup="true"
            aria-expanded={open}
            aria-controls={menuId}
            aria-label={t("inbox.moreActions", { name })}
            onClick={() => setOpen(!open)}
          >
            <span aria-hidden="true">•••</span>
          </Button>
          {open && (
            <ul id={menuId} className="inbox-menu">
              {order && (
                <>
                  <li>
                    <button
                      type="button"
                      disabled={order.position === 1}
                      onClick={() => {
                        setOpen(false);
                        order.onMove(order.position - 2);
                      }}
                    >
                      {t("inbox.moveUp")}
                    </button>
                  </li>
                  <li>
                    <button
                      type="button"
                      disabled={order.position === order.total}
                      onClick={() => {
                        setOpen(false);
                        order.onMove(order.position);
                      }}
                    >
                      {t("inbox.moveDown")}
                    </button>
                  </li>
                </>
              )}
              <li>
                <button
                  type="button"
                  className="danger"
                  onClick={() => {
                    setOpen(false);
                    onDelete();
                  }}
                >
                  {t("inbox.delete")}
                </button>
              </li>
            </ul>
          )}
        </div>
      </div>
    </li>
  );
}

/** A compact row for the "Book order" mode of a narrow screen: grip and arrows, no actions. */
export function OrderRow({ note, order }: { note: NoteItem; order: Order }) {
  const { t } = useTranslation();
  const name = noteName(note, t("inbox.anonymous"));
  return (
    <li className={`order-row ${dropClass(note.id, order)}`} data-note-id={note.id}>
      <button
        type="button"
        className="inbox-grip"
        {...gripProps(note.id, order, t("inbox.grip", { name }))}
      >
        <span aria-hidden="true">⠿</span>
      </button>
      <span className="inbox-pos">{order.position}</span>
      <span className="order-row-text">
        <strong>{name}</strong>
        <small>{note.answers.message}</small>
      </span>
      <Button
        variant="ghost"
        disabled={order.position === 1}
        aria-label={t("inbox.moveUpName", { name })}
        onClick={() => order.onMove(order.position - 2)}
      >
        <span aria-hidden="true">↑</span>
      </Button>
      <Button
        variant="ghost"
        disabled={order.position === order.total}
        aria-label={t("inbox.moveDownName", { name })}
        onClick={() => order.onMove(order.position)}
      >
        <span aria-hidden="true">↓</span>
      </Button>
    </li>
  );
}
