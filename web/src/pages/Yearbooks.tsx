import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { listYearbooks, mediaUrl, type Yearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";

export const YEARBOOKS_KEY = ["yearbooks"] as const;

function Card({ y, lang }: { y: Yearbook; lang: string }) {
  const meta = [y.school_name, y.class_name, y.graduation_year].filter(Boolean).join(" · ");
  return (
    <li className="card">
      {y.cover_media_id && <img src={mediaUrl(y.cover_media_id)} alt="" loading="lazy" />}
      <h2>
        <Link to={`/yearbooks/${y.id}`}>{y.title}</Link>
      </h2>
      {meta && <p>{meta}</p>}
      <p>
        <small>
          <time dateTime={y.updated_at}>
            {new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(new Date(y.updated_at))}
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
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  const books = q.data?.pages.flatMap((p) => p.yearbooks) ?? [];
  return (
    <>
      <h1>{t("books.title")}</h1>
      <p>
        <Link to="/yearbooks/new" className="button">
          {t("books.new")}
        </Link>
      </p>
      {q.isPending && <p role="status">{t("auth.loading")}</p>}
      {q.isError && (
        <>
          <p role="alert" className="error">
            {errorText(t, q.error)}
          </p>
          <button type="button" onClick={() => void q.refetch()}>
            {t("auth.retry")}
          </button>
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
        <button
          type="button"
          disabled={q.isFetchingNextPage}
          onClick={() => void q.fetchNextPage()}
        >
          {t("books.more")}
        </button>
      )}
      {q.isFetchNextPageError && (
        <p role="alert" className="error">
          {errorText(t, q.error)}
        </p>
      )}
    </>
  );
}
