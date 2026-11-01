import {
  useEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type CSSProperties,
  type DragEvent,
  type FocusEvent,
} from "react";
import { setText, type Refusal } from "./document";
import type { GestureStore } from "./gestures/useGestures";
import { fixText, plainText } from "./textInput";

/** Typing closer together than this is one history step; the step is also ended by blur, Escape and closing edit mode. */
export const TYPING_BURST_MS = 1000;

/**
 * The text field of a text box (T-161, ADR 0004 Q3), the one on the page and the one in the property panel.
 * - The browser does the typing. The value is a local draft that the field never rewrites while a composition runs (`compositionstart` to `compositionend`);
 *   outside one, every change is made plain and cut to 500 code points at once, with the caret put back where it was (a controlled field would send it to the end).
 *   At `compositionend` the value is cut the same way. `maxlength` is not used: it counts UTF-16 units.
 * - The text goes to the document as a preview of `setText` (one history step per burst of typing), so the page shows it; the step is committed on blur,
 *   on Escape, after a pause and when the field goes away. The field keeps the browser's own undo while it has the focus.
 * - Paste takes `text/plain` only; a drop of rich text is turned into its plain text.
 */
export function TextField({
  kind,
  id,
  value,
  store,
  label,
  className,
  style,
  rows,
  readOnly,
  focus,
  selectAll,
  onBlur,
  onEscape,
  onRefused,
}: {
  kind: string;
  id: string;
  /** The text in the document. */
  value: string;
  store: Pick<GestureStore, "preview" | "commit">;
  /** The accessible name. */
  label: string;
  className?: string;
  style?: CSSProperties;
  rows?: number;
  readOnly?: boolean;
  /** Take the focus when it appears (the caret at the end, or the whole text selected with `selectAll`). */
  focus?: boolean;
  selectAll?: boolean;
  /** After the typing so far is committed. */
  onBlur?: (e: FocusEvent<HTMLTextAreaElement>) => void;
  /** Escape was pressed (the typing so far is committed). */
  onEscape?: () => void;
  onRefused?: (why: Refusal | undefined) => void;
}) {
  const ta = useRef<HTMLTextAreaElement>(null);
  const composing = useRef(false);
  const open = useRef(false); // a burst of typing is a gesture of the store
  const timer = useRef(0);
  const [draft, setDraft] = useState(value);
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    // the document changed under us (the other field, an undo): show it
    setSeen(value);
    setDraft(value);
  }

  const commit = () => {
    clearTimeout(timer.current);
    if (!open.current) return;
    open.current = false;
    store.commit();
  };
  useEffect(
    () => () => {
      clearTimeout(timer.current);
      if (open.current) {
        open.current = false;
        store.commit();
      }
    },
    [store],
  );

  useEffect(() => {
    const el = ta.current;
    if (!focus || !el) return;
    el.focus({ preventScroll: true });
    el.setSelectionRange(selectAll ? 0 : el.value.length, el.value.length);
  }, [focus, selectAll]);

  const push = (text: string) => {
    const why = store.preview(setText(kind, id, { text }));
    onRefused?.(why);
    if (why) return;
    open.current = true;
    clearTimeout(timer.current);
    timer.current = window.setTimeout(commit, TYPING_BURST_MS);
  };
  /** Takes what the browser put in the field: made plain and cut to the cap, the caret kept where the user was typing. */
  const take = (el: HTMLTextAreaElement) => {
    const raw = el.value;
    const next = fixText(raw);
    if (next !== raw) {
      const at = Math.min(el.selectionStart, next.length);
      // React puts the value back after the handler, which sends the caret to the end; this runs after that
      queueMicrotask(() => el.setSelectionRange(at, at));
    }
    setDraft(next);
    push(next);
  };
  const insertPlain = (el: HTMLTextAreaElement, raw: string) => {
    const text = plainText(raw);
    // execCommand keeps the browser's own undo and the caret; setRangeText is the fallback where it is not there
    if (document.execCommand?.("insertText", false, text)) return;
    el.setRangeText(text, el.selectionStart, el.selectionEnd, "end");
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const paste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    e.preventDefault();
    insertPlain(e.currentTarget, e.clipboardData.getData("text/plain"));
  };
  const drop = (e: DragEvent<HTMLTextAreaElement>) => {
    const d = e.dataTransfer;
    if (!d.types.includes("text/html") && d.files.length === 0) return; // plain text: the browser inserts it, and onChange makes it fit
    e.preventDefault();
    insertPlain(e.currentTarget, d.getData("text/plain"));
  };

  return (
    <textarea
      ref={ta}
      className={className}
      style={style}
      aria-label={label}
      rows={rows}
      readOnly={readOnly}
      spellCheck={false}
      value={draft}
      onChange={(e) => (composing.current ? setDraft(e.target.value) : take(e.target))}
      onCompositionStart={() => {
        composing.current = true;
      }}
      onCompositionEnd={(e) => {
        composing.current = false;
        take(e.currentTarget);
      }}
      onPaste={paste}
      onDrop={drop}
      onKeyDown={(e) => {
        if (e.key !== "Escape" || e.nativeEvent.isComposing) return;
        e.preventDefault();
        e.stopPropagation();
        commit();
        onEscape?.();
      }}
      onBlur={(e) => {
        commit();
        onBlur?.(e);
      }}
    />
  );
}
