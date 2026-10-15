import type { Ref } from "react";
import { TextArea, TextField as BaseTextField } from "./ui/TextField";

type Props = {
  label: string;
  type?: "text" | "email" | "password" | "date" | "number";
  multiline?: boolean;
  value: string;
  onChange: (v: string) => void;
  autoComplete: string;
  error?: string | undefined;
  hint?: string;
  inputRef?: Ref<HTMLInputElement>; // lets a screen move focus here after a server error
};

/** A labelled input taking and giving plain strings; the error and hint are linked with aria-describedby. */
export function TextField({
  label,
  type = "text",
  multiline,
  value,
  onChange,
  autoComplete,
  error,
  hint,
  inputRef,
}: Props) {
  return multiline ? (
    <TextArea
      label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      autoComplete={autoComplete}
      error={error}
      help={hint}
    />
  ) : (
    <BaseTextField
      ref={inputRef}
      label={label}
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      autoComplete={autoComplete}
      error={error}
      help={hint}
    />
  );
}
