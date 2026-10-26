import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { setBook } from "../api/classes";
import { ApiError } from "../api/client";
import { listYearbooks } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { Button } from "../components/ui/Button";
import { YEARBOOKS_KEY } from "../pages/Yearbooks";
import { classErrorText } from "./classErrors";

/** Lists my own yearbooks and links the chosen one as my page. A 409 (linked elsewhere) is shown on its row, not hidden. */
export function PagePicker({
  classId,
  currentId,
  onDone,
  onCancel,
}: {
  classId: string;
  currentId: string | null | undefined;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useInfiniteQuery({
    queryKey: YEARBOOKS_KEY,
    queryFn: ({ pageParam }) => listYearbooks(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
  });
  // The list cannot tell which books are taken; the server says so on the 409, and the row keeps the reason.
  const [taken, setTaken] = useState<ReadonlySet<string>>(new Set());
  const pick = useMutation({
    mutationFn: (id: string) => setBook(classId, id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["class", classId] });
      onDone();
    },
    onError: (e, id) => {
      if (e instanceof ApiError && e.status === 409) setTaken((s) => new Set(s).add(id));
    },
  });
  const books = q.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <section aria-labelledby="pick-page">
      <h3 id="pick-page">{t("classes.pick.title")}</h3>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>
        </>
      )}
      {q.isSuccess && books.length === 0 && <p>{t("classes.pick.empty")}</p>}
      <ul className="member-list">
        {books.map((y) => {
          const here = y.id === currentId;
          const failed = pick.isError && pick.variables === y.id;
          return (
            <li key={y.id} className="card member-row">
              <strong>{y.title}</strong>
              {here ? (
                <p>{t("classes.pick.current")}</p>
              ) : taken.has(y.id) ? (
                <p role="alert" className="error">
                  {t("classes.errors.conflict")}
                </p>
              ) : (
                <>
                  {failed && (
                    <p role="alert" className="error">
                      {classErrorText(t, pick.error, "page")}
                    </p>
                  )}
                  <Button
                    busy={pick.isPending && pick.variables === y.id}
                    onClick={() => pick.mutate(y.id)}
                    aria-label={t("classes.pick.useFor", { title: y.title })}
                  >
                    {t("classes.pick.use")}
                  </Button>
                </>
              )}
            </li>
          );
        })}
      </ul>
      {q.hasNextPage && (
        <Button busy={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t("books.more")}
        </Button>
      )}
      <p>
        <Button onClick={onCancel}>{t("classes.cancel")}</Button>
      </p>
    </section>
  );
}
