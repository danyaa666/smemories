import type { TFunction } from "i18next";
import { ApiError } from "../api/client";
import { errorText } from "../auth/errors";

/** Where an error came from: the same code means different things in different forms (limit_reached is 10 classes, or 5 invite links). */
export type ClassCtx = "create" | "invite" | "role" | "join" | "leave" | "page" | "assemble";

/** `classes.errors.<ctx>.<code>`, then `classes.errors.<code>`, then the general message of that code. */
export function classErrorText(t: TFunction, e: unknown, ctx: ClassCtx): string {
  const general = errorText(t, e);
  if (!(e instanceof ApiError) || e.status === 0) return general;
  return t([`classes.errors.${ctx}.${e.code}`, `classes.errors.${e.code}`], {
    defaultValue: general,
  });
}

/** Field a class-form error belongs to. */
export const CLASS_FIELD_OF: Record<string, string> = {
  invalid_name: "name",
  invalid_school_name: "school",
  invalid_graduation_year: "year",
  invalid_language: "language",
};
