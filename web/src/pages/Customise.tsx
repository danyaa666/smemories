import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { getBookData, type BookData } from "../api/book";
import {
  getBookDesign,
  resetBookDesign,
  updateBookDesign,
  type BookDesign,
  type DesignBlock,
} from "../api/bookDesign";
import { retryOnce } from "../api/notes";
import { getYearbook } from "../api/yearbooks";
import { errorText, isBookGone } from "../auth/errors";
import { CustomiseControls } from "../book/CustomiseControls";
import { CustomisePreview } from "../book/CustomisePreview";
import {
  draftOf,
  messageField,
  optionsOf,
  resolvedOf,
  sameDraft,
  type Draft,
} from "../book/customise";
import { TemplatePicker } from "../book/TemplatePicker";
import { getTemplate } from "../book/templates";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { Skeleton } from "../components/ui/Skeleton";
import { useLeaveGuard } from "../components/useLeaveGuard";
import "../styles/customise.css";

/**
 * The customise screen (epic E09, T-144, decision D-41): the owner chooses colours, fonts, which sections print and in which order, the texts and the
 * pictures the template offers, sees the real book render change, and saves the validated document with `book-design/update`.
 * The owner check is the yearbook request the shell makes anyway: nothing else is requested for a book that is not the caller's.
 */
export function Customise() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
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
  // The design is read per template: choosing another template (the picker below) is another key, so the editor starts again from the new choices.
  const design = useQuery({
    queryKey: ["book-design", id, templateId],
    queryFn: () => getBookDesign(id),
    enabled: data.isSuccess,
    retry: retryOnce,
    refetchOnWindowFocus: false,
    staleTime: Infinity,
    gcTime: 0, // always read afresh when the screen opens: the editor starts from what is stored
  });
  const [firstTemplate, setFirstTemplate] = useState<string | null | undefined>(undefined);
  if (firstTemplate === undefined && data.data) setFirstTemplate(templateId);

  if (isBookGone(book.error)) return null; // the shell shows "book not found"
  const failed = book.error ?? data.error ?? design.error;
  return (
    <>
      <p>
        <Link to={`/yearbooks/${id}/book`}>{t("customise.back")}</Link>
      </p>
      <h1>{t("customise.title")}</h1>
      {failed ? (
        <Banner
          variant="error"
          action={
            <Button onClick={() => void (book.error ? book : data.error ? data : design).refetch()}>
              {t("auth.retry")}
            </Button>
          }
        >
          {errorText(t, failed)}
        </Banner>
      ) : !data.data || !design.data ? (
        <div role="status" aria-live="polite">
          <p>{t("customise.loading")}</p>
          <Skeleton width="100%" height={240} />
        </div>
      ) : (
        <>
          {firstTemplate !== undefined && firstTemplate !== templateId && (
            <Banner variant="warning">{t("customise.templateReset")}</Banner>
          )}
          {design.data.design ? (
            <Editor
              key={templateId ?? ""}
              bookId={id}
              data={data.data}
              info={design.data}
              block={design.data.design}
            />
          ) : (
            <>
              <Banner variant="warning">{t("customise.notAvailable")}</Banner>
              <TemplatePicker
                bookId={id}
                size={data.data.yearbook.page_size}
                current={templateId}
              />
              <Link className={buttonClass("secondary")} to={`/yearbooks/${id}/book`}>
                {t("customise.back")}
              </Link>
            </>
          )}
        </>
      )}
    </>
  );
}

function Editor({
  bookId,
  data,
  info,
  block,
}: {
  bookId: string;
  data: BookData;
  info: BookDesign;
  block: DesignBlock;
}) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const templateId = data.yearbook.template_id;
  const tpl = getTemplate(templateId);
  const bookT = useMemo(
    () => i18n.getFixedT(data.yearbook.language),
    [i18n, data.yearbook.language],
  ); // the book's own language, not the app's
  const [saved, setSaved] = useState<Draft>(() => draftOf(block, info.options));
  const [draft, setDraft] = useState<Draft>(saved);
  const [focus, setFocus] = useState<string | null>(null);
  const [status, setStatus] = useState(""); // visible, polite: saved, reset
  const [moves, setMoves] = useState(""); // spoken only: moves and switches
  const [asking, setAsking] = useState(false);
  const dirty = !sameDraft(draft, saved);
  const guard = useLeaveGuard(dirty);

  // After a save or a reset the print screen must draw the new design without fetching the book again.
  const applied = (next: Draft, options: BookDesign["options"], updatedAt: number | null) => {
    setSaved(next);
    setDraft(next);
    qc.setQueryData<BookData>(
      ["book-data", bookId],
      (d) => d && { ...d, design: resolvedOf(next) },
    );
    qc.setQueryData<BookDesign>(["book-design", bookId, templateId], {
      ...info,
      options,
      updated_at: updatedAt,
    });
  };
  const save = useMutation({
    mutationFn: () => updateBookDesign(bookId, optionsOf(draft)),
    onSuccess: (res) => {
      applied(draftOf(block, res.options), res.options, res.updated_at);
      setStatus(t("customise.saved"));
    },
  });
  const reset = useMutation({
    mutationFn: () => resetBookDesign(bookId),
    onSuccess: () => {
      applied(draftOf(block, null), null, null);
      setStatus(t("customise.resetDone"));
      setAsking(false);
    },
  });
  const edit = (change: (d: Draft) => Draft) => {
    save.reset();
    setStatus("");
    setDraft(change);
  };

  // A 400 ERROR_INVALID_DESIGN names the field in its message: it is shown on that text, else above the buttons.
  const invalid =
    save.error instanceof ApiError && save.error.code === "invalid_design" ? save.error : null;
  const field = invalid ? messageField(invalid.message) : null;
  const textErrors = invalid && field?.kind === "texts" ? { [field.id]: invalid.message } : {};

  return (
    <div className="cu">
      <div className="cu-side">
        <p>{t("customise.intro")}</p>
        <TemplatePicker
          bookId={bookId}
          size={data.yearbook.page_size}
          current={data.yearbook.template_id}
        />
        <CustomiseControls
          tpl={tpl}
          tplId={templateId ?? ""}
          block={block}
          draft={draft}
          edit={edit}
          bookT={bookT}
          textErrors={textErrors}
          announce={setMoves}
          onFocus={setFocus}
        />
        <div className="cu-actions">
          {invalid && !(field?.kind === "texts") && (
            <Banner variant="error">{invalid.message}</Banner>
          )}
          {save.isError && !invalid && <Banner variant="error">{errorText(t, save.error)}</Banner>}
          {reset.isError && !asking && <Banner variant="error">{errorText(t, reset.error)}</Banner>}
          <p role="status" aria-live="polite">
            {save.isPending
              ? t("customise.saving")
              : status || (dirty ? t("customise.unsaved") : "")}
          </p>
          <div className="cu-buttons">
            <Button
              variant="primary"
              busy={save.isPending}
              disabled={!dirty}
              onClick={() => save.mutate()}
            >
              {t("customise.save")}
            </Button>
            <Button
              disabled={save.isPending}
              onClick={() => {
                reset.reset();
                setAsking(true);
              }}
            >
              {t("customise.reset")}
            </Button>
          </div>
        </div>
        <div className="visually-hidden" role="status" aria-live="polite">
          {moves}
        </div>
      </div>
      <CustomisePreview data={data} draft={draft} block={block} focus={focus} bookT={bookT} />
      {asking && (
        <Dialog
          title={t("customise.resetTitle")}
          confirmLabel={t("customise.resetConfirm")}
          cancelLabel={t("customise.resetCancel")}
          danger
          busy={reset.isPending}
          onConfirm={() => reset.mutate()}
          onCancel={() => setAsking(false)}
        >
          <p>{t("customise.resetBody")}</p>
          {reset.isError && (
            <p role="alert" className="error">
              {errorText(t, reset.error)}
            </p>
          )}
        </Dialog>
      )}
      {guard.pending !== null && (
        <Dialog
          title={t("customise.leaveTitle")}
          confirmLabel={t("customise.leaveConfirm")}
          cancelLabel={t("customise.leaveCancel")}
          danger
          onConfirm={guard.leave}
          onCancel={guard.stay}
        >
          <p>{t("customise.leaveBody")}</p>
        </Dialog>
      )}
    </div>
  );
}
