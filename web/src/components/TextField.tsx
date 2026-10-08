import { useId, type ChangeEvent } from "react";

type Props = {
  label: string;
  type?: "text" | "email" | "password" | "date" | "number";
  multiline?: boolean;
  value: string;
  onChange: (v: string) => void;
  autoComplete: string;
  error?: string | undefined;
  hint?: string;
};

/** A labelled input; the error and hint are linked with aria-describedby. */
export function TextField({
  label,
  type = "text",
  multiline,
  value,
  onChange,
  autoComplete,
  error,
  hint,
}: Props) {
  const id = useId();
  const describedBy = [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(" ");
  const shared = {
    value,
    autoComplete,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": describedBy || undefined,
    onChange: (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => onChange(e.target.value),
  };
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {multiline ? (
        <textarea id={id} rows={3} {...shared} />
      ) : (
        <input id={id} type={type} {...shared} />
      )}
      {hint && <small id={`${id}-hint`}>{hint}</small>}
      {error && (
        <p id={`${id}-error`} role="alert" className="error">
          {error}
        </p>
      )}
    </div>
  );
}
