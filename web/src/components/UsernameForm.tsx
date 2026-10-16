import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { updateUsername, type User } from "../api/auth";
import { ApiError } from "../api/client";
import { ME_KEY } from "../auth/useMe";
import { errorText } from "../auth/errors";
import { TextField } from "./ui/TextField";
import { Button } from "./ui/Button";

// Mirrors the server rule (api/openapi.yaml, docs/usernames.md) for an instant hint only; the server decides.
const VALID = /^(?=.{3,30}$)[a-z0-9]+([._-][a-z0-9]+)*$/;

/** The account page's username block: the handle, when it may change again, and the change form. */
export function UsernameForm({ user }: { user: User }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [value, setValue] = useState("");
  const [touched, setTouched] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const note = useRef<HTMLParagraphElement>(null);
  // The pending button is disabled and drops focus, so each outcome says where focus goes next.
  const focusAfter = useRef<"input" | "note" | null>(null);

  const when = (ms: number) =>
    new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(ms);
  const next = user.username_next_change_at;
  // ponytail: "now" is read once at mount, so a cooldown that ends while the page stays open unlocks on reload
  const [now] = useState(Date.now);
  const waiting = next !== null && next > now;

  const m = useMutation({
    mutationFn: updateUsername,
    onSuccess: (u) => {
      focusAfter.current = "note";
      qc.setQueryData(ME_KEY, u);
    },
    onError: (e) => {
      focusAfter.current = "input";
      if (!(e instanceof ApiError)) return;
      if (e.status === 401) {
        qc.setQueryData(ME_KEY, null); // the session is gone: RequireAuth sends the user to sign in
      } else if (e.code === "username_change_too_soon" && e.retryAfter) {
        // the last change happened elsewhere (another tab or device): lock the form until the server's time
        focusAfter.current = "note";
        qc.setQueryData(ME_KEY, {
          ...user,
          username_next_change_at: Date.now() + e.retryAfter * 1000,
        });
      }
    },
  });

  useEffect(() => {
    const el = focusAfter.current === "note" ? note.current : input.current;
    if (m.isPending || !focusAfter.current || !el) return;
    el.focus();
    focusAfter.current = null;
  });

  const lowered = value.trim().toLowerCase();
  const tooSoon =
    m.error instanceof ApiError && m.error.code === "username_change_too_soon" && waiting;
  const serverError = m.error && !tooSoon ? errorText(t, m.error) : undefined;
  const error =
    (touched || m.isError) && lowered !== "" && !VALID.test(lowered)
      ? t("errors.invalid_username")
      : serverError;
  const message = tooSoon
    ? t("errors.username_change_too_soon", { date: when(next) })
    : [
        m.isSuccess && t("account.usernameSaved"),
        waiting && t("account.usernameCooldown", { date: when(next) }),
      ]
        .filter(Boolean)
        .join(" ");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (m.isPending || waiting) return;
    setTouched(true);
    if (!VALID.test(lowered)) {
      input.current?.focus();
      return;
    }
    m.mutate(lowered);
  };

  return (
    <section aria-labelledby="username-title" className="username-block">
      <h2 id="username-title">{t("account.username")}</h2>
      <p className="username-current">{user.username}</p>
      {message && (
        <p ref={note} tabIndex={-1} role={tooSoon ? "alert" : "status"}>
          {message}
        </p>
      )}
      <form onSubmit={submit} noValidate>
        <TextField
          ref={input}
          label={t("account.usernameNew")}
          help={t("account.usernameHelp")}
          error={error}
          value={value}
          onChange={(e) => setValue(e.target.value.toLowerCase())}
          onBlur={() => setTouched(true)}
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          disabled={waiting}
        />
        <Button type="submit" variant="primary" busy={m.isPending} disabled={waiting}>
          {m.isPending ? t("auth.working") : t("account.usernameSave")}
        </Button>
      </form>
    </section>
  );
}
