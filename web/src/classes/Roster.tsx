import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { listMembers, removeMember, updateRole, type ClassMember } from "../api/classes";
import { errorText } from "../auth/errors";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { classErrorText } from "./classErrors";

/** Admin only: the roster with member actions. Mounted for admins alone, so a student never requests it. */
export function Roster({ classId }: { classId: string }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const key = ["class-members", classId];
  const q = useInfiniteQuery({
    queryKey: key,
    queryFn: ({ pageParam }) => listMembers(classId, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
  });
  const [removing, setRemoving] = useState<ClassMember | null>(null);
  const done = () => void qc.invalidateQueries({ queryKey: key });
  const role = useMutation({
    mutationFn: (m: ClassMember) =>
      updateRole(classId, m.id, m.role === "admin" ? "student" : "admin"),
    onSuccess: done,
  });
  const remove = useMutation({
    mutationFn: (m: ClassMember) => removeMember(classId, m.id),
    onSuccess: () => {
      setRemoving(null);
      done();
    },
  });
  const members = q.data?.pages.flatMap((p) => p.items) ?? [];
  const date = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium" });
  return (
    <section aria-labelledby="roster">
      <h2 id="roster">{t("classes.roster.title")}</h2>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>
        </>
      )}
      {role.error && (
        <p role="alert" className="error">
          {classErrorText(t, role.error, "role")}
        </p>
      )}
      <ul className="member-list">
        {members.map((m) => (
          <li key={m.id} className="card member-row">
            <strong>{m.name}</strong>
            <p>
              {t("classes.roster.role")}: {t(`classes.role.${m.role}`)} ·{" "}
              {t("classes.roster.joined", { date: date.format(new Date(m.joined_at)) })}
            </p>
            <p>
              {m.page_linked
                ? `${t(m.profile_complete ? "classes.page.complete" : "classes.page.incomplete")} · ${t("classes.page.notes", { n: m.approved_notes })}`
                : t("classes.roster.noPage")}
            </p>
            <div className="row-actions">
              <Button
                busy={role.isPending && role.variables?.id === m.id}
                onClick={() => role.mutate(m)}
                aria-label={t(
                  m.role === "admin"
                    ? "classes.roster.makeStudentFor"
                    : "classes.roster.makeAdminFor",
                  { name: m.name },
                )}
              >
                {t(m.role === "admin" ? "classes.roster.makeStudent" : "classes.roster.makeAdmin")}
              </Button>
              <Button
                variant="danger"
                onClick={() => {
                  remove.reset();
                  setRemoving(m);
                }}
                aria-label={t("classes.roster.removeFor", { name: m.name })}
              >
                {t("classes.roster.remove")}
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {q.hasNextPage && (
        <Button busy={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t("books.more")}
        </Button>
      )}
      {removing && (
        <Dialog
          title={t("classes.roster.removeTitle", { name: removing.name })}
          confirmLabel={t("classes.roster.remove")}
          cancelLabel={t("classes.cancel")}
          danger
          busy={remove.isPending}
          onConfirm={() => remove.mutate(removing)}
          onCancel={() => setRemoving(null)}
        >
          <p>{t("classes.roster.removeBody")}</p>
          {remove.error && (
            <p role="alert" className="error">
              {classErrorText(t, remove.error, "role")}
            </p>
          )}
        </Dialog>
      )}
    </section>
  );
}
