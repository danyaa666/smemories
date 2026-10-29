import { useQuery } from "@tanstack/react-query";
import { flushSync } from "react-dom";
import { useCallback, useEffect, useMemo, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { getClassBook, type ClassBook as ClassBookData } from "../api/classBook";
import { retryOnce } from "../api/notes";
import { errorText } from "../auth/errors";
import { AdminOnly } from "../classes/AdminOnly";
import { LeftOutNotice, leftOut } from "../classes/LeftOutNotice";
import { ClassBookPages } from "../book/ClassBookPages";
import {
  PAGE_BUDGET,
  classPageCount,
  classSummary,
  commonSize,
  memberBook,
  memberPhotoIds,
} from "../book/classBook";
import { useFitReport } from "../book/fit";
import { PrintGuide } from "../book/PrintGuide";
import { useFontsLoaded, usePhotoPreload } from "../book/ready";
import { SIZES, pageCss, type PageSize } from "../book/sizes";
import "../book/book.css";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { Skeleton } from "../components/ui/Skeleton";
import { useStageScale } from "./Book";

/** Preview and print of the class book (E07, T-130): the admin's view of every student's approved pages, printed by the browser. */
export function ClassBook() {
  return <AdminOnly>{(classId) => <ClassBookView id={classId} />}</AdminOnly>;
}

function ClassBookView({ id }: { id: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ["class-book", id],
    queryFn: () => getClassBook(id),
    retry: retryOnce, // a 4xx (403 for a student) is the answer; a network error or 5xx gets one more try
    refetchOnWindowFocus: false, // coming back from the print dialog must not reload up to 2 MB
    structuralSharing: false,
  });
  return (
    <>
      <h1 className="bk-ui">{t("classBook.title")}</h1>
      {q.isPending && (
        <div className="bk-ui" role="status" aria-live="polite">
          <p>{t("classBook.loading")}</p>
          <Skeleton width="100%" height={240} />
        </div>
      )}
      {q.isError && (
        <div className="bk-ui">
          <Banner
            variant="error"
            action={
              <Button onClick={() => void q.refetch()} busy={q.isFetching}>
                {t("auth.retry")}
              </Button>
            }
          >
            {errorText(t, q.error)}
          </Banner>
          <Link className={buttonClass("secondary")} to={`/classes/${id}`}>
            {t("classBook.back")}
          </Link>
        </div>
      )}
      {q.data && <ClassBookScreen book={q.data} classId={id} />}
    </>
  );
}

const isSize = (s: string | null): s is PageSize => s !== null && Object.hasOwn(SIZES, s);

function ClassBookScreen({ book, classId }: { book: ClassBookData; classId: string }) {
  const { t, i18n } = useTranslation();
  const [params, setParams] = useSearchParams();
  const wanted = params.get("size");
  const size = isSize(wanted) ? wanted : commonSize(book.members);
  const bookT = useMemo(() => i18n.getFixedT(book.class.language), [i18n, book.class.language]);
  const photoList = useMemo(
    () => [
      ...new Set(
        book.members.flatMap((m) => {
          const d = memberBook(book.class, m, size);
          return d ? memberPhotoIds(d) : [];
        }),
      ),
    ],
    [book, size],
  );
  const photos = usePhotoPreload(photoList);
  const pages = classPageCount(book, size);
  const fonts = useFontsLoaded(`${book.class.id}|${book.members.length}|${size}|${pages}`);
  const { states, Provider } = useFitReport();
  const { ref, scale } = useStageScale(size);
  const [eager, setEager] = useState(false);
  const [loading, setLoading] = useState(0); // photos on the pages that have not settled yet
  const track = useCallback((delta: number) => setLoading((n) => n + delta), []);
  // Notes left out of the answer (8 MiB cap): printing needs the admin's yes, asked again if the list changes.
  const cutKey = leftOut(book)
    .map((m) => m.member_id)
    .join(",");
  const [acked, setAcked] = useState("");
  const ackOk = cutKey === "" || acked === cutKey;
  const ready = fonts && photos.settled && loading === 0;

  // The print dialog lays the page out as soon as the event handler returns: load every image first.
  useEffect(() => {
    const before = () => flushSync(() => setEager(true));
    const after = () => setEager(false);
    window.addEventListener("beforeprint", before);
    window.addEventListener("afterprint", after);
    return () => {
      window.removeEventListener("beforeprint", before);
      window.removeEventListener("afterprint", after);
    };
  }, []);

  // For the automated print check (tools/print-check): set when printing is allowed.
  useEffect(() => {
    if (!ready) return;
    document.documentElement.dataset.bookReady = String(pages);
    return () => {
      delete document.documentElement.dataset.bookReady;
    };
  }, [ready, pages]);

  const summary = useMemo(() => classSummary(book, size, states), [book, size, states]);
  const stageStyle = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": scale,
  } as CSSProperties;
  const nameOf = (n: string) => n.trim() || t("classBook.unnamed");

  return (
    <>
      <style>{pageCss(size)}</style>
      <div className="bk-ui">
        <p>
          {book.class.name} · {size}
        </p>
        <fieldset className="bk-templates">
          <legend>{t("classBook.size")}</legend>
          <div className="bk-template-list">
            {(Object.keys(SIZES) as PageSize[]).map((s) => (
              <label key={s}>
                <input
                  type="radio"
                  name="class-book-size"
                  checked={s === size}
                  onChange={() => setParams({ size: s }, { replace: true })}
                />{" "}
                {s}
              </label>
            ))}
          </div>
          <p className="bk-template-help">{t("classBook.sizeHelp")}</p>
        </fieldset>
        {pages > PAGE_BUDGET && (
          <Banner variant="warning">
            {t("classBook.overBudget", { pages, budget: PAGE_BUDGET })}
          </Banner>
        )}
        {photos.failed > 0 && (
          <Banner variant="warning">{t("print.photosFailed", { count: photos.failed })}</Banner>
        )}
        <LeftOutNotice
          book={book}
          acked={acked === cutKey}
          onAck={(v) => setAcked(v ? cutKey : "")}
        />
        {summary.noPage.length > 0 && (
          <Banner variant="warning">
            {t("classBook.noPage", {
              count: summary.noPage.length,
              names: summary.noPage.map((m) => nameOf(m.name)).join(", "),
            })}
          </Banner>
        )}
        {summary.coverCut && <Banner variant="warning">{t("classBook.coverCut")}</Banner>}
        {summary.cutPages.length > 0 && (
          <Banner variant="warning">
            {t("classBook.cutPages", {
              count: summary.cutPages.length,
              titles: summary.cutPages.map((p) => p.title).join(", "),
            })}
          </Banner>
        )}
        {summary.issues.length > 0 && (
          <details className="bk-fits" open>
            <summary>{t("classBook.issuesTitle", { count: summary.issues.length })}</summary>
            <p>{t("classBook.issuesHelp")}</p>
            <ul>
              {summary.issues.map((i) => (
                <li key={i.id}>
                  {nameOf(i.name)}:{" "}
                  {[
                    i.dropped > 0 && t("classBook.dropped", { shown: i.shown, total: i.total }),
                    i.cutNotes > 0 && t("classBook.cutNotes", { count: i.cutNotes }),
                    i.cutProfile > 0 && t("classBook.cutProfile", { count: i.cutProfile }),
                  ]
                    .filter(Boolean)
                    .join(" ")}
                </li>
              ))}
            </ul>
          </details>
        )}
        <p role="status">
          {ready
            ? t("print.ready", { pages })
            : t("print.preparing", { done: photos.loaded + photos.failed, total: photos.total })}
        </p>
        <Button variant="primary" disabled={!ready || !ackOk} onClick={() => window.print()}>
          {t("print.print")}
        </Button>
        <PrintGuide size={size} />
        <Link className={buttonClass("secondary")} to={`/classes/${classId}`}>
          {t("classBook.back")}
        </Link>
      </div>
      <div ref={ref} className="bk-stage" style={stageStyle}>
        <Provider>
          {/* Another paper size is a new layout: the text fit starts again. */}
          <ClassBookPages
            key={size}
            book={book}
            size={size}
            t={bookT}
            eager={eager}
            track={track}
          />
        </Provider>
      </div>
    </>
  );
}
