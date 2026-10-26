import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  createInvite,
  listInvites,
  revokeInvite,
  type ClassInvite,
  type ClassInviteCreated,
} from "../api/classes";
import { errorText } from "../auth/errors";
import { SelectField } from "../components/SelectField";
import { Badge } from "../components/ui/Badge";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { classErrorText } from "./classErrors";

const DAY = 24 * 60 * 60 * 1000;
const CHOICES = ["7", "14", "30"] as const;
// A minute short of the choice: the server accepts at most 30 days from *its* now, and our clock may run ahead.
const expiry = (days: string) => Date.now() + Number(days) * DAY - 60_000;

/**
 * Admin only. The new link lives in this component's state and nowhere else (not in a URL, a store or the invite list),
 * so a reload or a navigation drops it for good: the server cannot show it again.
 */
export function Invites({ classId }: { classId: string }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const key = ["class-invites", classId];
  const q = useInfiniteQuery({
    queryKey: key,
    queryFn: ({ pageParam }) => listInvites(classId, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
  });
  const [days, setDays] = useState<string>("14");
  const [fresh, setFresh] = useState<ClassInviteCreated | null>(null);
  const [copied, setCopied] = useState<"ok" | "failed" | null>(null);
  const [revoking, setRevoking] = useState<ClassInvite | null>(null);
  const create = useMutation({
    mutationFn: () => createInvite(classId, expiry(days)),
    onSuccess: (inv) => {
      setFresh(inv);
      setCopied(null);
      void qc.invalidateQueries({ queryKey: key });
    },
  });
  const revoke = useMutation({
    mutationFn: (inv: ClassInvite) => revokeInvite(classId, inv.id),
    onSuccess: () => {
      setRevoking(null);
      void qc.invalidateQueries({ queryKey: key });
    },
  });
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(fresh?.link ?? "");
      setCopied("ok");
    } catch {
      setCopied("failed");
    }
  };
  const hide = () => {
    setFresh(null);
    create.reset(); // the mutation result holds the link too
  };
  const invites = q.data?.pages.flatMap((p) => p.items) ?? [];
  const date = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium" });
  const state = (i: ClassInvite) =>
    i.active ? "active" : i.revoked_at !== null ? "revoked" : "expired";
  return (
    <section aria-labelledby="invites">
      <h2 id="invites">{t("classes.invites.title")}</h2>
      <p>{t("classes.invites.help")}</p>
      <SelectField
        label={t("classes.invites.lifetime")}
        value={days}
        onChange={setDays}
        options={CHOICES.map((d) => ({ value: d, label: t("classes.invites.days", { n: d }) }))}
      />
      {create.error && (
        <p role="alert" className="error">
          {classErrorText(t, create.error, "invite")}
        </p>
      )}
      <Button variant="primary" busy={create.isPending} onClick={() => create.mutate()}>
        {t("classes.invites.create")}
      </Button>
      {fresh && (
        <div className="invite-link">
          <Banner variant="warning">{t("classes.invites.once")}</Banner>
          <div className="field">
            <label htmlFor="invite-link">{t("classes.invites.link")}</label>
            <input
              id="invite-link"
              readOnly
              value={fresh.link}
              autoComplete="off"
              onFocus={(e) => e.currentTarget.select()}
            />
          </div>
          <div className="row-actions">
            <Button onClick={() => void copy()}>{t("classes.invites.copy")}</Button>
            <Button variant="ghost" onClick={hide}>
              {t("classes.invites.hide")}
            </Button>
          </div>
          <p role="status">
            {copied === "ok" && t("classes.invites.copied")}
            {copied === "failed" && t("classes.invites.copyFailed")}
          </p>
        </div>
      )}
      <h3>{t("classes.invites.listTitle")}</h3>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>
        </>
      )}
      {q.isSuccess && invites.length === 0 && <p>{t("classes.invites.empty")}</p>}
      <ul className="member-list">
        {invites.map((i) => (
          <li key={i.id} className="card member-row">
            <Badge variant={i.active ? "approved" : "hidden"}>
              {t(`classes.invites.state.${state(i)}`)}
            </Badge>
            <p>
              {t("classes.invites.created", { date: date.format(new Date(i.created_at)) })} ·{" "}
              {t("classes.invites.expires", { date: date.format(new Date(i.expires_at)) })}
            </p>
            {i.active && (
              <Button
                variant="danger"
                onClick={() => {
                  revoke.reset();
                  setRevoking(i);
                }}
              >
                {t("classes.invites.revoke")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {q.hasNextPage && (
        <Button busy={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t("books.more")}
        </Button>
      )}
      {revoking && (
        <Dialog
          title={t("classes.invites.revokeTitle")}
          confirmLabel={t("classes.invites.revoke")}
          cancelLabel={t("classes.cancel")}
          danger
          busy={revoke.isPending}
          onConfirm={() => revoke.mutate(revoking)}
          onCancel={() => setRevoking(null)}
        >
          <p>{t("classes.invites.revokeBody")}</p>
          {revoke.error && (
            <p role="alert" className="error">
              {classErrorText(t, revoke.error, "invite")}
            </p>
          )}
        </Dialog>
      )}
    </section>
  );
}
