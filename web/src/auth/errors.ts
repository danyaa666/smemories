import type { TFunction } from "i18next";
import { ApiError } from "../api/client";

// Errors that belong to one input; everything else is shown above the submit button.
const FIELD_OF: Record<string, string> = {
  invalid_email: "email",
  weak_password: "password",
  invalid_display_name: "display_name",
  invalid_code: "code",
  code_expired: "code",
  code_locked: "code",
  // yearbook and profile forms: the field is named like the code
  ...Object.fromEntries(
    [
      "title",
      "school_name",
      "class_name",
      "graduation_year",
      "motto",
      "full_name",
      "nickname",
      "birthday",
      "quote",
      "hobbies",
      "future_plans",
    ].map((f) => [`invalid_${f}`, f]),
  ),
};

export function errorText(t: TFunction, e: unknown): string {
  if (!(e instanceof ApiError)) return t("errors.generic");
  if (e.status === 0) return t("errors.network_error");
  const minutes = Math.max(1, Math.ceil((e.retryAfter ?? 60) / 60));
  return t(`errors.${e.code}`, { minutes, defaultValue: t("errors.generic") });
}

/** The message for `field` if `e` belongs to it. */
export const fieldError = (t: TFunction, e: unknown, field: string) =>
  e instanceof ApiError && FIELD_OF[e.code] === field ? errorText(t, e) : undefined;

/** The message for errors that belong to no input. */
export const formError = (t: TFunction, e: unknown) =>
  e && !(e instanceof ApiError && e.code in FIELD_OF) ? errorText(t, e) : undefined;

/** Only same-site paths: one leading slash, no `//` or `/\`. */
export const safePath = (p: unknown): string =>
  typeof p === "string" && /^\/(?![/\\])/.test(p) ? p : "/account";
