import { useState } from "react";
import { useTranslation } from "react-i18next";

export const tenth = (v: number) => String(Math.round(v * 10) / 10);
/** A number as the language writes it (vi: decimal comma), one decimal at most (F-10). */
export const shown = (lng: string, v: number) =>
  new Intl.NumberFormat(lng, { maximumFractionDigits: 1, useGrouping: false }).format(v);
/** What the user typed, with a decimal point or a decimal comma; NaN when it is not a plain number. */
export const typed = (s: string) => {
  const t = s.trim().replace(",", ".");
  return /^-?(\d+\.?\d*|\.\d+)$/.test(t) ? Number(t) : NaN; // no '0x10', '1e1', '+5', 'Infinity'
};

/**
 * A number that is committed on Enter or when it loses the focus: one history step per committed input. `onCommit` returns false when the change was refused.
 * An empty, non-numeric or out-of-range entry puts the previous value back and says which range is allowed (R4).
 */
export function NumField({
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
  min: number;
  max: number;
  disabled: boolean;
  onCommit: (n: number) => boolean;
}) {
  const { t, i18n } = useTranslation();
  const show = (v: number) => shown(i18n.language, v);
  const [text, setText] = useState(show(value));
  const [bad, setBad] = useState(false);
  const lo = Math.ceil(min * 10) / 10; // what the input can show, so the message and the check agree
  const hi = Math.floor(max * 10) / 10;
  const commit = () => {
    const n = typed(text);
    const valid = text.trim() !== "" && Number.isFinite(n) && n >= lo && n <= hi;
    setBad(!valid);
    if (!valid || Math.abs(n - value) < 0.05) return setText(show(value));
    if (!onCommit(n)) setText(show(value));
  };
  return (
    <>
      <label className="ed-field">
        <span>{label}</span>
        <input
          type="text" // not "number": a number input writes the separator of the browser, not of the page language
          role="spinbutton"
          inputMode="decimal"
          aria-valuemin={lo}
          aria-valuemax={hi}
          aria-valuenow={value}
          value={text}
          disabled={disabled}
          aria-invalid={bad || undefined}
          onChange={(e) => {
            setText(e.target.value);
            setBad(false);
          }}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") return commit();
            const up = e.key === "ArrowUp";
            if (!up && e.key !== "ArrowDown") return;
            e.preventDefault(); // what a number input does: a step, clamped, written back as text until Enter or blur
            const n = typed(text);
            const from = Number.isFinite(n) && text.trim() !== "" ? n : value;
            setText(show(Math.min(hi, Math.max(lo, from + (up ? step : -step)))));
          }}
        />
      </label>
      {bad && (
        <p role="alert" className="ed-field-error">
          {t("editor.range", { min: show(lo), max: show(hi) })}
        </p>
      )}
    </>
  );
}
