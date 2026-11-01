import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { OnePage } from "../BookPages";
import type { PageBox, Rect } from "../layout/geometry";
import {
  clearOverride,
  ops,
  removeItem,
  reorderItem,
  setBg,
  setOverride,
  type LayoutDoc,
  type Op,
  type Refusal,
} from "./document";
import { pageName, targetName } from "./names";
import { overrideOf, patchOf, type Target } from "./targets";

const tenth = (v: number) => String(Math.round(v * 10) / 10);

/** A number that is committed on Enter or when it loses the focus: one history step per committed input. `onCommit` returns false when the change was refused. */
function NumField({
  label,
  value,
  step,
  min,
  max,
  disabled,
  onCommit,
}: {
  label: string;
  value: number;
  step: number;
  min?: number;
  max?: number;
  disabled: boolean;
  onCommit: (n: number) => boolean;
}) {
  const [text, setText] = useState(tenth(value));
  const commit = () => {
    const n = Number(text);
    if (text.trim() === "" || !Number.isFinite(n) || Math.abs(n - value) < 0.05)
      return setText(tenth(value));
    if (!onCommit(n)) setText(tenth(value));
  };
  return (
    <label className="ed-field">
      <span>{label}</span>
      <input
        type="number"
        inputMode="decimal"
        step={step}
        min={min}
        max={max}
        value={text}
        disabled={disabled}
        onChange={(e) => setText(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => e.key === "Enter" && commit()}
      />
    </label>
  );
}

/**
 * The property panel (T-159): the numbers, the stacking order and the actions of the selection, or the properties of the page when nothing is selected.
 * Every change goes through the T-158 store as one history step (`apply`). A template element is always written with its whole box (taken from where it
 * is drawn), so an edit of one number does not move the others back to the template's own place.
 */
export function PropertyPanel({
  doc,
  kind,
  page,
  pb,
  selected,
  apply,
  select,
  readOnly,
  announce,
}: {
  doc: LayoutDoc;
  kind: string;
  page: OnePage;
  pb: PageBox;
  selected: Target | undefined;
  apply: (op: Op) => Refusal | undefined;
  select: (ids: readonly string[]) => void;
  readOnly: boolean;
  announce: (s: string) => void;
}) {
  const { t } = useTranslation();
  const [last, setRefused] = useState<{ id?: string; why: Refusal } | null>(null);
  const refused = last && last.id === selected?.id ? last.why : null; // a refusal belongs to the selection it was made on

  const run = (op: Op) => {
    const r = apply(op);
    setRefused(r ? { id: selected?.id, why: r } : null);
    return !r;
  };

  if (!selected) {
    const bg = doc.pages[kind]?.bg;
    return (
      <aside className="ed-panel" aria-label={t("editor.panel")}>
        <h2>{t("editor.pageProps", { page: pageName(t, page) })}</h2>
        <p className="ed-hint">{t("editor.nothingSelected")}</p>
        {bg && (
          <button type="button" disabled={readOnly} onClick={() => run(setBg(kind, null))}>
            {t("editor.clearBackground")}
          </button>
        )}
        {refused && (
          <p role="alert">
            {t(`editor.refused.${refused}`, { defaultValue: t("editor.refused.other") })}
          </p>
        )}
      </aside>
    );
  }

  const item = selected.item;
  const write = (rect: Rect, r: number) => {
    const patch = patchOf(rect, r, pb);
    return run(
      item ? ops.setItemBox(selected.kind, selected.id, patch) : setOverride(selected.id, patch),
    );
  };
  const rect = selected.rect;
  const field = (
    label: string,
    value: number,
    make: (n: number) => [Rect, number],
    step = 0.1,
    min?: number,
    max?: number,
  ) => (
    <NumField
      key={`${label}:${tenth(value)}`}
      label={label}
      value={value}
      step={step}
      min={min}
      max={max}
      disabled={readOnly}
      onCommit={(n) => write(...make(n))}
    />
  );
  const list = doc.pages[kind]?.items ?? [];
  const at = item ? list.findIndex((i) => i.id === item.id) : -1;
  const move = (to: number) => {
    if (run(reorderItem(kind, selected.id, to)))
      announce(t("editor.reordered", { n: to + 1, total: list.length }));
  };
  const hasOverride = overrideOf(doc, selected.id) !== undefined;

  return (
    <aside className="ed-panel" aria-label={t("editor.panel")}>
      <h2>{targetName(t, selected)}</h2>
      <fieldset disabled={readOnly}>
        <legend>{t("editor.box")}</legend>
        {field(t("editor.x"), (rect.left / pb.w) * 100, (n) => [
          { ...rect, left: (n / 100) * pb.w },
          selected.r,
        ])}
        {field(t("editor.y"), (rect.top / pb.h) * 100, (n) => [
          { ...rect, top: (n / 100) * pb.h },
          selected.r,
        ])}
        {field(t("editor.w"), (rect.width / pb.w) * 100, (n) => [
          { ...rect, width: (n / 100) * pb.w },
          selected.r,
        ])}
        {field(t("editor.h"), (rect.height / pb.h) * 100, (n) => [
          { ...rect, height: (n / 100) * pb.h },
          selected.r,
        ])}
        {field(t("editor.rotation"), selected.r, (n) => [rect, Math.round(n)], 1, -180, 180)}
      </fieldset>
      {refused && (
        <p role="alert">
          {t(`editor.refused.${refused}`, { defaultValue: t("editor.refused.other") })}
        </p>
      )}
      {item ? (
        <>
          {item.t === "text" && (
            <div>
              <h3>{t("editor.text")}</h3>
              <p className="ed-preview">{item.text}</p>
            </div>
          )}
          <div role="group" aria-label={t("editor.order")} className="ed-row">
            <button
              type="button"
              disabled={readOnly || at >= list.length - 1}
              onClick={() => move(list.length - 1)}
            >
              {t("editor.toFront")}
            </button>
            <button
              type="button"
              disabled={readOnly || at >= list.length - 1}
              onClick={() => move(at + 1)}
            >
              {t("editor.forward")}
            </button>
            <button type="button" disabled={readOnly || at <= 0} onClick={() => move(at - 1)}>
              {t("editor.backward")}
            </button>
            <button type="button" disabled={readOnly || at <= 0} onClick={() => move(0)}>
              {t("editor.toBack")}
            </button>
          </div>
          <button
            type="button"
            className="ed-danger"
            disabled={readOnly}
            onClick={() => {
              if (run(removeItem(kind, selected.id))) {
                select([]);
                announce(t("editor.deleted"));
              }
            }}
          >
            {t("editor.delete")}
          </button>
        </>
      ) : (
        <div className="ed-row">
          <button
            type="button"
            disabled={readOnly}
            onClick={() => run(setOverride(selected.id, { hidden: !selected.hidden }))}
          >
            {selected.hidden ? t("editor.show") : t("editor.hide")}
          </button>
          <button
            type="button"
            disabled={readOnly || !hasOverride}
            onClick={() => run(clearOverride(selected.id))}
          >
            {t("editor.resetElement")}
          </button>
        </div>
      )}
    </aside>
  );
}
