import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { getBookData } from "../api/book";
import { getBookDesign } from "../api/bookDesign";
import { retryOnce } from "../api/notes";
import { getYearbook } from "../api/yearbooks";
import { errorText, isBookGone } from "../auth/errors";
import { EditorShell } from "../book/editor/EditorShell";
import { getTemplate } from "../book/templates";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { Skeleton } from "../components/ui/Skeleton";
import { useMediaQuery } from "../components/useMediaQuery";

const DEFAULT_SAFE_MM = 5; // the guide when the design block cannot be read: it is only a drawing aid

/**
 * The free-layout editor route (epic E09 stage 2, T-159): `/yearbooks/:id/editor`, owner only (the yearbook request the shell makes anyway answers 403/404
 * for anyone else, and nothing else is requested then). Below 768 px the editor does not mount at all: a message says so (phone mode is a later task).
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
      {narrow ? <Banner variant="warning">{t("editor.narrow")}</Banner> : <Loaded id={id} />}
    </>
  );
}

function Loaded({ id }: { id: string }) {
  const { t } = useTranslation();
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
    <EditorShell
      key={templateId ?? ""}
      bookId={id}
      data={data.data}
      safeMm={design.data?.design?.layout?.safe_margin_mm ?? DEFAULT_SAFE_MM}
    />
  );
}
