import {
  useId,
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

export function TextField({ label, help, error, counter, ...input }: InputProps) {
  const id = useId();
  const c = { label, help, error, counter, max: input.maxLength };
  return (
    <Shell {...c} id={id} length={runes(input.value)}>
      <input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(id, c)}
        {...input}
      />
    </Shell>
  );
}

type AreaProps = Common &
  Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "value"> & {
    value: string;
    ref?: Ref<HTMLTextAreaElement>;
  };

export function TextArea({ label, help, error, counter, rows = 3, ...area }: AreaProps) {
  const id = useId();
  const c = { label, help, error, counter, max: area.maxLength };
  return (
    <Shell {...c} id={id} length={runes(area.value)}>
      <textarea
        id={id}
        rows={rows}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(id, c)}
        {...area}
      />
    </Shell>
  );
}
