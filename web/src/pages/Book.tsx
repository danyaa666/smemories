import { useQuery } from "@tanstack/react-query";
import { flushSync } from "react-dom";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { getBookData, type BookData } from "../api/book";
import { retryOnce } from "../api/notes";
import { errorText } from "../auth/errors";
import { BookPages, pageCount } from "../book/BookPages";
import { useFitReport } from "../book/fit";
import { noteFits, ownFits } from "../book/fitReport";
import { getTemplate } from "../book/templates";
import { photoIds, useFontsLoaded, usePhotoPreload } from "../book/ready";
import { SIZES, pageCss, pagePx } from "../book/sizes";
import "../book/book.css";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { Skeleton } from "../components/ui/Skeleton";

const MAX_SCALE = 1.5; // on a wide screen a page is shown up to 1.5 times its paper size

/** Preview and print of the whole book (decision D-24): one request, fixed-size pages, the browser's own print dialog. */
export function Book() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const q = useQuery({
    queryKey: ["book-data", id],
    queryFn: () => getBookData(id),
    retry: retryOnce, // a 4xx is the answer; a network error or 5xx gets one more try
    refetchOnWindowFocus: false, // coming back from the print dialog must not reload up to several MB
    structuralSharing: false,
  });
  return (
    <>
      <h1 className="bk-ui">{t("print.title")}</h1>
      {q.isPending && (
        <div className="bk-ui" role="status" aria-live="polite">
          <p>{t("print.loading")}</p>
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
          <Link className={buttonClass("secondary")} to={`/yearbooks/${id}`}>
            {t("print.back")}
          </Link>
        </div>
      )}
      {q.data && <BookScreen data={q.data} bookId={id} />}
    </>
  );
}

/** Pages scaled to the width of the stage on screen, never above MAX_SCALE. */
function useStageScale(size: keyof typeof SIZES) {
  const ref = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => {
      const w = el.clientWidth;
      if (w > 0) setScale(Math.round(Math.min(MAX_SCALE, w / pagePx(size).w) * 1000) / 1000);
    };
    update();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [size]);
  return { ref, scale };
}

function BookScreen({ data, bookId }: { data: BookData; bookId: string }) {
  const { t, i18n } = useTranslation();
  const size = data.yearbook.page_size;
  const language = data.yearbook.language;
  const bookT = useMemo(() => i18n.getFixedT(language), [i18n, language]); // the book's own language, not the app's
  const photos = usePhotoPreload(
    useMemo(() => photoIds(data, getTemplate(data.yearbook.template_id).photos), [data]),
  );
  const key = `${data.yearbook.title}|${data.note_count}|${size}`;
  const fonts = useFontsLoaded(key);
  const { states, Provider } = useFitReport();
  const { ref, scale } = useStageScale(size);
  const [eager, setEager] = useState(false);
  const ready = fonts && photos.settled;
  const pages = pageCount(data);

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

  const fits = useMemo(() => noteFits(states, data), [states, data]);
  const cutNotes = fits.filter((f) => f.cut).length;
  const { cut: cutOther, smaller } = ownFits(states);
  const stageStyle = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": scale,
  } as CSSProperties;

  return (
    <>
      {/* One @page rule for the whole document: every page has the size of the book and no margin. */}
      <style>{pageCss(size)}</style>
      <div className="bk-ui">
        <p>
          {data.yearbook.title} · {size}
        </p>
        {data.truncated && (
          <Banner variant="warning">
            {t("print.truncated", { shown: data.note_count, total: data.approved_total })}
          </Banner>
        )}
        {data.note_count === 0 && <Banner variant="warning">{t("print.noNotes")}</Banner>}
        {photos.failed > 0 && (
          <Banner variant="warning">{t("print.photosFailed", { count: photos.failed })}</Banner>
        )}
        {cutNotes > 0 && (
          <Banner variant="warning">{t("print.cutNotes", { count: cutNotes })}</Banner>
        )}
        {cutOther > 0 && (
          <Banner variant="warning">{t("print.cutOther", { count: cutOther })}</Banner>
        )}
        {smaller > 0 && <p>{t("print.smaller", { count: smaller })}</p>}
        {fits.length > 0 && (
          <details className="bk-fits" open={cutNotes > 0 || fits.length <= 10}>
            <summary>{t("print.fitTitle", { count: fits.length })}</summary>
            <p>{t("print.fitHelp")}</p>
            <ul>
              {fits.map((f) => {
                const name = f.name || t("print.fitNoName", { number: f.number });
                return (
                  <li key={f.id}>
                    {t(f.cut ? "print.fitCut" : "print.fitSmaller", { name, pt: f.pt })}{" "}
                    <Link
                      to={`/yearbooks/${bookId}/notes?note=${encodeURIComponent(f.id)}`}
                      aria-label={t("print.fitOpenFor", { name })}
                    >
                      {t("print.fitOpen")}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </details>
        )}
        <p role="status">
          {ready
            ? t("print.ready", { pages })
            : t("print.preparing", { done: photos.loaded + photos.failed, total: photos.total })}
        </p>
        <Button variant="primary" disabled={!ready} onClick={() => window.print()}>
          {t("print.print")}
        </Button>
        <details>
          <summary>{t("print.helpTitle")}</summary>
          <ol>
            <li>{t("print.help1")}</li>
            <li>{t("print.help2", { size })}</li>
            <li>{t("print.help3")}</li>
            <li>{t("print.help4")}</li>
          </ol>
        </details>
      </div>
      <div ref={ref} className="bk-stage" style={stageStyle}>
        <Provider>
          <BookPages data={data} size={size} t={bookT} eager={eager} />
        </Provider>
      </div>
    </>
  );
}
