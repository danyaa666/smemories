import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { dropClass, gripProps, type DragState, type Order } from "../components/NoteCard";
import { Button } from "../components/ui/Button";
import { arrayMove } from "./ordering";

export type OrderItem = {
  id: string;
  name: string;
  meta?: string;
  body?: string;
  extra?: ReactNode;
};

const CTLS = ["grip", "up", "down"];

/**
 * One reorderable list: drag by the grip, ArrowUp/ArrowDown on the grip, or the move buttons (the same pattern as the notes
 * order). After a move, focus returns to the control that was used (or the nearest enabled one when it just went to an end),
 * and `announce` tells a live region the new position.
 */
export function OrderList({
  labelledBy,
  items,
  onChange,
  announce,
}: {
  labelledBy: string;
  items: OrderItem[];
  onChange: (ids: string[]) => void;
  announce: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [drag, setDrag] = useState<DragState>(null);
  const list = useRef<HTMLOListElement>(null);
  const [refocus, setRefocus] = useState<{ id: string; ctl: string } | null>(null); // a new object per move
  useEffect(() => {
    const r = refocus;
    if (!r) return;
    // a moved DOM node loses focus in some browsers, so put it back by hand
    const row = [...(list.current?.querySelectorAll<HTMLElement>("[data-note-id]") ?? [])].find(
      (el) => el.dataset.noteId === r.id,
    );
    const order = [r.ctl, ...CTLS.filter((c) => c !== r.ctl)];
    for (const c of order) {
      const b = row?.querySelector<HTMLButtonElement>(`[data-ctl="${c}"]`);
      if (b && !b.disabled) return b.focus();
    }
  }, [refocus]);

  function move(from: number, to: number) {
    const item = items[from];
    if (!item || from === to || to < 0 || to >= items.length) return;
    const active = document.activeElement;
    setRefocus({
      id: item.id,
      ctl: (active instanceof HTMLElement && active.dataset.ctl) || "grip",
    });
    onChange(
      arrayMove(
        items.map((i) => i.id),
        from,
        to,
      ),
    );
    announce(t("assemble.moved", { name: item.name, position: to + 1, total: items.length }));
  }
  const indexOf = (id: string) => items.findIndex((x) => x.id === id);
  const orderFor = (item: OrderItem, i: number): Order => ({
    position: i + 1,
    total: items.length,
    dragging: drag,
    onMove: (to) => move(i, to),
    onDragStart: () => setDrag({ id: item.id, over: item.id }),
    onDragOver: (over) => setDrag((d) => (d && d.over !== over ? { ...d, over } : d)),
    onDragEnd: (commit) => {
      if (commit && drag && drag.over !== drag.id) move(indexOf(drag.id), indexOf(drag.over));
      setDrag(null);
    },
    indexOf,
  });

  return (
    <ol ref={list} className="order-list" aria-labelledby={labelledBy}>
      {items.map((item, i) => {
        const order = orderFor(item, i);
        return (
          <li
            key={item.id}
            className={`order-row assemble-row ${dropClass(item.id, order)}`}
            data-note-id={item.id}
          >
            <button
              type="button"
              className="inbox-grip"
              data-ctl="grip"
              {...gripProps(item.id, order, t("assemble.grip", { name: item.name }))}
            >
              <span aria-hidden="true">⠿</span>
            </button>
            <span className="inbox-pos">{i + 1}</span>
            <span className="order-row-text">
              <strong>{item.name}</strong>
              {item.meta && <small>{item.meta}</small>}
            </span>
            <Button
              variant="ghost"
              data-ctl="up"
              disabled={i === 0}
              aria-label={t("assemble.moveUp", { name: item.name })}
              onClick={() => move(i, i - 1)}
            >
              <span aria-hidden="true">↑</span>
            </Button>
            <Button
              variant="ghost"
              data-ctl="down"
              disabled={i === items.length - 1}
              aria-label={t("assemble.moveDown", { name: item.name })}
              onClick={() => move(i, i + 1)}
            >
              <span aria-hidden="true">↓</span>
            </Button>
            {item.body !== undefined && <PageBody text={item.body} clamp />}
            {item.extra}
          </li>
        );
      })}
    </ol>
  );
}

/** A class page body as text: one React text node, newlines kept by CSS. Never markup. */
export function PageBody({ text, clamp }: { text: string; clamp?: boolean }) {
  return <p className={clamp ? "page-body page-body-clamp" : "page-body"}>{text}</p>;
}
