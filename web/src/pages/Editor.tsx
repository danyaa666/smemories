import { useQuery, useQueryClient } from "@tanstack/react-query";
import { lazy } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { getBookData } from "../api/book";
import { getBookDesign } from "../api/bookDesign";
import { retryOnce } from "../api/notes";
import { getYearbook } from "../api/yearbooks";
import { errorText, isBookGone } from "../auth/errors";
import { EditorBody } from "../book/editor/EditorBody";
import { PhoneStage } from "../book/editor/PhoneStage";
import { getTemplate } from "../book/templates";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { Skeleton } from "../components/ui/Skeleton";
import { useMediaQuery } from "../components/useMediaQuery";

// The desktop stage holds every gesture module (drag, resize, rotate, snap, keys): it is a chunk of its own, fetched only at 768 px and wider (T-181).
const DesktopStage = lazy(() => import("../book/editor/Stage").then((m) => ({ default: m.Stage })));

const DEFAULT_SAFE_MM = 5; // the guide when the design block cannot be read: it is only a drawing aid

/**
 * The free-layout editor route (epic E09 stage 2, T-159): `/yearbooks/:id/editor`, owner only (the yearbook request the shell makes anyway answers 403/404
 * for anyone else, and nothing else is requested then). Below 768 px it is the phone editor (T-181: view and light edit through a bottom sheet); the one
 * component and store serve both widths, so crossing 768 px keeps the selection, the open sheet and the typed text.
 */
export function Editor() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const narrow = useMediaQuery("(max-width: 767px)");
  return (
    <>
      <p>
        <Link to={`/yearbooks/${id}/customise`}>{t("editor.back")}</Link>
      </p>
      <h1>{t("editor.title")}</h1>
      <Loaded id={id} phone={narrow} />
    </>
  );
}

function Loaded({ id, phone }: { id: string; phone: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const book = useQuery({ queryKey: ["yearbook", id], queryFn: () => getYearbook(id) }); // the same request as the shell's: it tells who owns the book
  const data = useQuery({
    queryKey: ["book-data", id],
    queryFn: () => getBookData(id),
    enabled: book.isSuccess,
    retry: retryOnce,
    refetchOnWindowFocus: false,
    structuralSharing: false,
  });
  const templateId = data.data?.yearbook.template_id ?? null;
  // only the safe margin is read from the design block; a failure here must not stop the editor
  const design = useQuery({
    queryKey: ["book-design", id, templateId],
    queryFn: () => getBookDesign(id),
    enabled: data.isSuccess,
    retry: false,
    refetchOnWindowFocus: false,
    staleTime: Infinity,
  });

  if (isBookGone(book.error)) return null; // the shell shows "book not found"
  const failed = book.error ?? data.error;
  if (failed)
    return (
      <Banner
        variant="error"
        action={
          <Button onClick={() => void (book.error ? book : data).refetch()}>
            {t("auth.retry")}
          </Button>
        }
      >
        {errorText(t, failed)}
      </Banner>
    );
  if (!data.data || (data.isSuccess && design.isPending))
    return (
      <div role="status" aria-live="polite">
        <p>{t("editor.loading")}</p>
        <Skeleton width="100%" height={240} />
      </div>
    );
  if (!getTemplate(data.data.yearbook.template_id).layout)
    return <Banner variant="warning">{t("editor.noLayout")}</Banner>;
  return (
    <EditorBody
      key={templateId ?? ""}
      phone={phone}
      Stage={phone ? PhoneStage : DesktopStage}
      bookId={id}
      onDone={() => void qc.invalidateQueries({ queryKey: ["book-data", id] })} // the manage page shows the book as it is now
      data={data.data}
      safeMm={design.data?.design?.layout?.safe_margin_mm ?? DEFAULT_SAFE_MM}
    />
  );
}
