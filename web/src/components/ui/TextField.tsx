import {
  useId,
  type ChangeEvent,
  type InputHTMLAttributes,
  type ReactNode,
  type Ref,
  type TextareaHTMLAttributes,
} from "react";

type Common = {
  label: string;
  help?: string | undefined;
  /** Shown under the field, linked with aria-describedby and announced (role="alert"). */
  error?: string | undefined;
  /** Show "n/max" under the field; needs maxLength. */
  counter?: boolean;
};

/** The server counts characters (runes), not UTF-16 units: an emoji is 1, not 2. */
const runes = (s: string) => [...s].length;

/** First `max` characters; spreading a string walks code points, so a surrogate pair is never split. */
export const cutRunes = (s: string, max: number) =>
  runes(s) > max ? [...s].slice(0, max).join("") : s;

/** Native maxLength counts UTF-16 units: a field with a counter does not get it, and the limit is enforced here, by character. */
function byRune<E extends HTMLInputElement | HTMLTextAreaElement>(
  max: number | undefined,
  onChange: ((e: ChangeEvent<E>) => void) | undefined,
) {
  if (max === undefined) return onChange;
  return (e: ChangeEvent<E>) => {
    e.target.value = cutRunes(e.target.value, max); // a paste is cut at the limit before the handler reads it
    onChange?.(e);
  };
}

type Parts = Common & { max?: number | undefined };

function Shell({
  id,
  label,
  help,
  error,
  counter,
  max,
  length,
  children,
}: Parts & { id: string; length: number; children: ReactNode }) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children}
      {help && (
        <small id={`${id}-help`} className="field-help">
          {help}
        </small>
      )}
      {counter && max !== undefined && (
        <small id={`${id}-count`} className="field-count">
          {length}/{max}
        </small>
      )}
      {error && (
        <p id={`${id}-error`} role="alert" className="field-error">
          {error}
        </p>
      )}
    </div>
  );
}

const describedBy = (id: string, c: Parts) =>
  [
    c.help && `${id}-help`,
    c.counter && c.max !== undefined && `${id}-count`,
    c.error && `${id}-error`,
  ]
    .filter(Boolean)
    .join(" ") || undefined;

type InputProps = Common &
  Omit<InputHTMLAttributes<HTMLInputElement>, "value"> & {
    value: string;
    ref?: Ref<HTMLInputElement>;
  };

export function TextField({
  label,
  help,
  error,
  counter,
  maxLength,
  onChange,
  ...input
}: InputProps) {
  const id = useId();
  const c = { label, help, error, counter, max: maxLength };
  return (
    <Shell {...c} id={id} length={runes(input.value)}>
      <input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(id, c)}
        {...input}
        maxLength={counter ? undefined : maxLength}
        onChange={byRune(counter ? maxLength : undefined, onChange)}
      />
    </Shell>
  );
}

type AreaProps = Common &
  Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "value"> & {
    value: string;
    ref?: Ref<HTMLTextAreaElement>;
  };

export function TextArea({
  label,
  help,
  error,
  counter,
  rows = 3,
  maxLength,
  onChange,
  ...area
}: AreaProps) {
  const id = useId();
  const c = { label, help, error, counter, max: maxLength };
  return (
    <Shell {...c} id={id} length={runes(area.value)}>
      <textarea
        id={id}
        rows={rows}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(id, c)}
        {...area}
        maxLength={counter ? undefined : maxLength}
        onChange={byRune(counter ? maxLength : undefined, onChange)}
      />
    </Shell>
  );
}
