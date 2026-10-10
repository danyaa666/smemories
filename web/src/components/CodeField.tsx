import { useEffect, useId, useRef, type ClipboardEvent, type ChangeEvent } from "react";

export const CODE_LENGTH = 6;

type Props = {
  label: string;
  hint?: string;
  value: string;
  onChange: (v: string) => void;
  /** Called when the sixth digit has just been entered (typed, pasted or autofilled). */
  onComplete?: (v: string) => void;
  error?: string | undefined;
  disabled?: boolean;
  focusOnMount?: boolean;
};

const digits = (s: string) => s.replace(/\D/g, "").slice(0, CODE_LENGTH);

/** The one-time code input: digits only, pasting "123 456" works, the browser may autofill it from the email app. */
export function CodeField({
  label,
  hint,
  value,
  onChange,
  onComplete,
  error,
  disabled,
  focusOnMount,
}: Props) {
  const id = useId();
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (focusOnMount) ref.current?.focus();
  }, [focusOnMount]);
  useEffect(() => {
    if (error) ref.current?.focus(); // the error is linked to the field, so the reader hears both
  }, [error]);
  const set = (raw: string) => {
    const v = digits(raw);
    onChange(v);
    if (v.length === CODE_LENGTH && v !== value) onComplete?.(v);
  };
  const describedBy = [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(" ");
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <input
        ref={ref}
        id={id}
        type="text"
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={CODE_LENGTH}
        spellCheck={false}
        disabled={disabled}
        value={value}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy || undefined}
        onChange={(e: ChangeEvent<HTMLInputElement>) => set(e.target.value)}
        onPaste={(e: ClipboardEvent<HTMLInputElement>) => {
          e.preventDefault(); // maxLength would cut "123 456" to "123 45" before we can clean it
          set(e.clipboardData.getData("text"));
        }}
      />
      {hint && <small id={`${id}-hint`}>{hint}</small>}
      {error && (
        <p id={`${id}-error`} role="alert" className="error">
          {error}
        </p>
      )}
    </div>
  );
}
