import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  createLink,
  listLinks,
  retryOnce,
  revokeLink,
  shareUrl,
  type NoteCollection,
} from "../api/notes";
import { errorText } from "../auth/errors";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { EmptyState } from "../components/ui/EmptyState";
import { TextField } from "../components/ui/TextField";

const MAX_ACTIVE = 5;
const LABEL_MAX = 60;
const DAY_MS = 86_400_000;

const pad = (n: number) => String(n).padStart(2, "0");
const dateValue = (ms: number) => {
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
/** The picked day, until its last millisecond in the owner's time zone. */
const endOfDay = (value: string) => {
  const [y, m, d] = value.split("-").map(Number);
  return new Date(y!, m! - 1, d!, 23, 59, 59, 999).getTime();
};

type Shown = { token: string; label: string };

export function NoteLinks() {
  const { id = "" } = useParams();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const key = ["note-links", id] as const;
  const list = useQuery({
    queryKey: key,
    queryFn: () => listLinks(id),
    // a 4xx (not your book) is the answer; a network error or 5xx gets one more try
    retry: retryOnce,
  });
  const [label, setLabel] = useState("");
  const [day, setDay] = useState("");
  const [shown, setShown] = useState<Shown>();
  const [copied, setCopied] = useState(false);
  const [revoking, setRevoking] = useState<NoteCollection>();
  const [now] = useState(Date.now); // when the page opened: enough to tell open links from closed ones

  const links = list.data?.items ?? [];
  const active = links.filter((l) => !l.revoked_at).length;

  const create = useMutation({
    mutationFn: () => createLink(id, label.trim(), day ? endOfDay(day) : null),
    onSuccess: (r) => {
      // the token is shown here once and kept nowhere else: not in the cache, not in storage
      setShown({ token: r.token, label: r.label });
      setCopied(false);
      setLabel("");
      setDay("");
      void qc.invalidateQueries({ queryKey: key });
    },
  });
  const revoke = useMutation({
    mutationFn: (linkId: string) => revokeLink(linkId),
    onSuccess: () => {
      setRevoking(undefined);
      void qc.invalidateQueries({ queryKey: key });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!create.isPending) create.mutate();
  };
  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      setCopied(false); // the link is in a read-only field the owner can select by hand
    }
  };

  const err = create.error;
  const code = err instanceof ApiError ? err.code : "";
  const labelError = code === "invalid_label" ? errorText(t, err) : undefined;
  const dayError = code === "invalid_deadline" ? errorText(t, err) : undefined;
  const formError =
    err && !labelError && !dayError
      ? code === "limit_reached"
        ? t("links.limitReached", { max: MAX_ACTIVE })
        : errorText(t, err)
      : undefined;

  function status(l: NoteCollection): { variant: "approved" | "pending" | "hidden"; text: string } {
    if (l.revoked_at) return { variant: "hidden", text: t("links.status.revoked") };
    if (l.deadline_at !== null && l.deadline_at <= now)
      return { variant: "pending", text: t("links.status.closed") };
    return { variant: "approved", text: t("links.status.open") };
  }

  return (
    <>
      <p>
        <Link to={`/yearbooks/${id}/notes`}>{t("links.back")}</Link>
      </p>
      <h1>{t("links.title")}</h1>
      <p>{t("links.intro")}</p>

      {shown && (
        <section className="links-share" aria-labelledby="links-share-title">
          <h2 id="links-share-title">{t("links.created")}</h2>
          <p>{t("links.once")}</p>
          <TextField
            label={shown.label ? t("links.linkNamed", { label: shown.label }) : t("links.link")}
            value={shareUrl(shown.token)}
            readOnly
            onFocus={(e) => e.currentTarget.select()}
          />
          <Button variant="primary" onClick={() => void copy(shareUrl(shown.token))}>
            {t("links.copy")}
          </Button>
          <span role="status" className="links-copied">
            {copied ? t("links.copied") : ""}
          </span>
          <Button variant="ghost" onClick={() => setShown(undefined)}>
            {t("links.done")}
          </Button>
        </section>
      )}

      <form onSubmit={submit} className="links-form" noValidate>
        <h2>{t("links.new")}</h2>
        <TextField
          label={t("links.label")}
          help={t("links.labelHelp")}
          value={label}
          maxLength={LABEL_MAX}
          counter
          error={labelError}
          onChange={(e) => setLabel(e.target.value)}
        />
        <TextField
          label={t("links.deadline")}
          help={t("links.deadlineHelp")}
          type="date"
          value={day}
          min={dateValue(now)}
          max={dateValue(now + 364 * DAY_MS)}
          error={dayError}
          onChange={(e) => setDay(e.target.value)}
        />
        {formError && (
          <p role="alert" className="error">
            {formError}
          </p>
        )}
        <Button
          type="submit"
          variant="primary"
          busy={create.isPending}
          disabled={active >= MAX_ACTIVE}
        >
          {t("links.create")}
        </Button>
        {active >= MAX_ACTIVE && (
          <p className="field-help">{t("links.limitReached", { max: MAX_ACTIVE })}</p>
        )}
      </form>

      <h2>{t("links.existing")}</h2>
      {list.isPending && <p role="status">{t("auth.loading")}</p>}
      {list.isError && (
        <EmptyState
          title={t("links.loadError")}
          action={<Button onClick={() => void list.refetch()}>{t("auth.retry")}</Button>}
        >
          {errorText(t, list.error)}
        </EmptyState>
      )}
      {list.isSuccess && links.length === 0 && <p>{t("links.none")}</p>}
      {links.length > 0 && (
        <ul className="links-list">
          {links.map((l) => {
            const s = status(l);
            return (
              <li key={l.id} className="card">
                <div className="links-row">
                  <strong>{l.label || t("links.unnamed")}</strong>
                  <Badge variant={s.variant}>{s.text}</Badge>
                </div>
                <small>
                  {[
                    t("links.notes", { count: l.note_count }),
                    t("links.createdOn", {
                      date: new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium" }).format(
                        l.created_at,
                      ),
                    }),
                    l.deadline_at !== null
                      ? t("links.until", {
                          date: new Intl.DateTimeFormat(i18n.language, {
                            dateStyle: "medium",
                          }).format(l.deadline_at),
                        })
                      : t("links.noDeadline"),
                  ].join(" · ")}
                </small>
                {!l.revoked_at && (
                  <Button variant="secondary" onClick={() => setRevoking(l)}>
                    {t("links.revoke")}{" "}
                    <span className="visually-hidden">{l.label || t("links.unnamed")}</span>
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {revoking && (
        <Dialog
          title={t("links.revokeTitle")}
          confirmLabel={t("links.revoke")}
          cancelLabel={t("inbox.cancel")}
          danger
          busy={revoke.isPending}
          onConfirm={() => revoke.mutate(revoking.id)}
          onCancel={() => setRevoking(undefined)}
        >
          <p>{t("links.revokeBody")}</p>
          {revoke.isError && (
            <p role="alert" className="error">
              {errorText(t, revoke.error)}
            </p>
          )}
        </Dialog>
      )}
    </>
  );
}
