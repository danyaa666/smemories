import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { listClasses, type SchoolClass } from "../api/classes";
import { errorText } from "../auth/errors";
import { Button, buttonClass } from "../components/ui/Button";

export const CLASSES_KEY = ["classes"] as const;

function Card({ c }: { c: SchoolClass }) {
  const { t } = useTranslation();
  const meta = [c.school_name, c.graduation_year].filter(Boolean).join(" · ");
  return (
    <li className="card">
      <h2>
        <Link to={`/classes/${c.id}`}>{c.name}</Link>
      </h2>
      {meta && <p>{meta}</p>}
      <p>
        {t("classes.roster.role")}: {t(`classes.role.${c.my_role}`)}
      </p>
    </li>
  );
}

export function Classes() {
  const { t } = useTranslation();
  const q = useInfiniteQuery({
    queryKey: CLASSES_KEY,
    queryFn: ({ pageParam }) => listClasses(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <>
      <h1>{t("classes.title")}</h1>
      <p>
        <Link to="/classes/new" className={buttonClass("primary")}>
          {t("classes.new")}
        </Link>
      </p>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>
        </>
      )}
      {q.isSuccess && items.length === 0 && <p>{t("classes.empty")}</p>}
      {items.length > 0 && (
        <ul className="cards">
          {items.map((c) => (
            <Card key={c.id} c={c} />
          ))}
        </ul>
      )}
      {q.hasNextPage && (
        <Button busy={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t("books.more")}
        </Button>
      )}
    </>
  );
}
