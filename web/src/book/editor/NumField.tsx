import { useState } from "react";
import { useTranslation } from "react-i18next";

export const tenth = (v: number) => String(Math.round(v * 10) / 10);

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
  const { t } = useTranslation();
  const [text, setText] = useState(tenth(value));
  const [bad, setBad] = useState(false);
  const lo = Math.ceil(min * 10) / 10; // what the input can show, so the message and the check agree
  const hi = Math.floor(max * 10) / 10;
  const commit = () => {
    const n = Number(text);
    const valid = text.trim() !== "" && Number.isFinite(n) && n >= lo && n <= hi;
    setBad(!valid);
    if (!valid || Math.abs(n - value) < 0.05) return setText(tenth(value));
    if (!onCommit(n)) setText(tenth(value));
  };
  return (
    <>
      <label className="ed-field">
        <span>{label}</span>
        <input
          type="number"
          inputMode="decimal"
          step={step}
          min={lo}
          max={hi}
          value={text}
          disabled={disabled}
          aria-invalid={bad || undefined}
          onChange={(e) => {
            setText(e.target.value);
            setBad(false);
          }}
          onBlur={commit}
          onKeyDown={(e) => e.key === "Enter" && commit()}
        />
      </label>
      {bad && (
        <p role="alert" className="ed-field-error">
          {t("editor.range", { min: tenth(lo), max: tenth(hi) })}
        </p>
      )}
    </>
  );
}
