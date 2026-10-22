import { useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from "react";
import { Button } from "./Button";

type Props = {
  title: string;
  children?: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  /** A destructive confirm: the confirm button is red. Focus starts on Cancel either way. */
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

const FOCUSABLE = "button:not([disabled]), a[href], input:not([disabled]), select, textarea";

/** A modal confirm. Render it only while it is open: it takes focus on mount, keeps Tab inside, closes on Escape and gives focus back on unmount. */
export function Dialog({
  title,
  children,
  confirmLabel,
  cancelLabel,
  danger,
  busy,
  onConfirm,
  onCancel,
}: Props) {
  const titleId = useId();
  const box = useRef<HTMLDivElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const before = document.activeElement;
    cancel.current?.focus(); // the safe choice first
    return () => {
      if (!(before instanceof HTMLElement)) return;
      before.focus();
      // A button that starts the work the dialog confirmed is disabled while it runs, and a disabled button cannot take focus
      // (it would fall to <body>): wait until it is enabled again, unless the user has moved on in the meantime.
      if (document.activeElement === before || !before.isConnected) return;
      const stop = () => {
        watch.disconnect();
        document.removeEventListener("focusin", stop);
        window.clearTimeout(giveUp);
      };
      const watch = new MutationObserver(() => {
        if (before.hasAttribute("disabled")) return;
        stop();
        if (document.activeElement === document.body) before.focus();
      });
      watch.observe(before, { attributes: true, attributeFilter: ["disabled"] });
      document.addEventListener("focusin", stop);
      const giveUp = window.setTimeout(stop, 30_000); // ponytail: a job longer than this leaves focus on <body>
    };
  }, []);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      onCancel();
    } else if (e.key === "Tab") {
      const items = [...(box.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [])];
      const first = items[0];
      const last = items[items.length - 1];
      if (!first || !last) return;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  };
  return (
    // A click outside the box cancels; the keyboard has Escape, so the backdrop needs no role.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      className="dialog-backdrop"
      onMouseDown={(e) => {
        if (e.target !== e.currentTarget) return;
        e.preventDefault(); // else the browser moves focus to <body> after the dialog is gone, undoing the focus return
        onCancel();
      }}
    >
      {/* The keydown handler is the focus trap; the dialog itself is not a control. */}
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
      <div
        ref={box}
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={onKeyDown}
      >
        <h2 id={titleId}>{title}</h2>
        {children && <div className="dialog-body">{children}</div>}
        <div className="dialog-actions">
          <Button ref={cancel} variant="secondary" onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button variant={danger ? "danger" : "primary"} busy={busy} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
