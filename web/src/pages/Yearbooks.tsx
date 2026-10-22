import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { QueuedImg } from "../components/QueuedImg";
import { listYearbooks, mediaUrl, type Yearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { Button, buttonClass } from "../components/ui/Button";

export const YEARBOOKS_KEY = ["yearbooks"] as const;

function Card({ y, lang }: { y: Yearbook; lang: string }) {
  const updated = new Date(y.updated_at);
  const meta = [y.school_name, y.class_name, y.graduation_year].filter(Boolean).join(" · ");
  return (
    <li className="card">
      {y.cover_media_id && <QueuedImg src={mediaUrl(y.cover_media_id)} alt="" />}
      <h2>
        <Link to={`/yearbooks/${y.id}`}>{y.title}</Link>
      </h2>
      {meta && <p>{meta}</p>}
      <p>
        <small>
          <time dateTime={updated.toISOString()}>
            {new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(updated)}
          </time>
        </small>
      </p>
    </li>
  );
}

export function Yearbooks() {
  const { t, i18n } = useTranslation();
  const q = useInfiniteQuery({
    queryKey: YEARBOOKS_KEY,
    queryFn: ({ pageParam }) => listYearbooks(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
  });
  const books = q.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <>
      <h1>{t("books.title")}</h1>
      <p>
        <Link to="/yearbooks/new" className={buttonClass("primary")}>
          {t("books.new")}
        </Link>
      </p>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <Button type="button" onClick={() => void q.refetch()}>
            {t("auth.retry")}
          </Button>
        </>
      )}
      {q.isSuccess && books.length === 0 && <p>{t("books.empty")}</p>}
      {books.length > 0 && (
        <ul className="cards">
          {books.map((y) => (
            <Card key={y.id} y={y} lang={i18n.language} />
          ))}
        </ul>
      )}
      {q.hasNextPage && (
        <Button busy={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t("books.more")}
        </Button>
      )}
      {q.isFetchNextPageError && (
        <p role="alert" className="error">
          {errorText(t, q.error)}
        </p>
      )}
    </>
  );
}
