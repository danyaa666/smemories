import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { deleteOwnTemplate, listPageTemplates } from "../../api/bookLayout";
import type { BookData } from "../../api/book";
import { ApiError } from "../../api/client";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { BookPages, type OnePage } from "../BookPages";
import type { SectionId } from "../design";
import { pagePx, SIZES, type PageSize } from "../sizes";
import { withPage, type LayoutDoc } from "./document";
import {
  allCards,
  groupTemplates,
  MAX_THUMBS,
  remember,
  templateError,
  type Lists,
  type TemplateCard,
  type TemplateError,
} from "./templates";

const THUMB_W = 96; // screen px

/** The id a card has in `previewId`: a shipped id and an own ULID could never meet, but the source makes it plain. */
export const cardKey = (c: Pick<TemplateCard, "source" | "id">) => `${c.source}:${c.id}`;

/** The text of a failed template call; `seconds` is the Retry-After of a 429. */
export function useTemplateErrorText() {
  const { t } = useTranslation();
  return (error: TemplateError, seconds?: number) =>
    t(`editor.templates.err.${error}`, { seconds: seconds ?? 5 });
}

const noop = () => {};
const pageOf = (kind: SectionId): OnePage =>
  kind === "notes" ? { section: kind, index: 0 } : { section: kind };

/** The template's page drawn by the book's own components on the book's real data, read-only (no handles, no focus). */
const Thumb = memo(function Thumb({
  data,
  doc,
  kind,
  card,
  size,
  bookT,
  drawn,
}: {
  data: BookData;
  doc: LayoutDoc;
  kind: SectionId;
  card: TemplateCard;
  size: PageSize;
  bookT: TFunction;
  drawn: boolean;
}) {
  const shown = useMemo(
    () => (drawn ? { ...data, layout: withPage(doc, kind, card.page) } : null),
    [drawn, data, doc, kind, card.page],
  );
  const style = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": THUMB_W / pagePx(size).w,
  } as CSSProperties;
  return (
    <span className="ed-thumb-view ed-tpl-thumb" style={style} aria-hidden="true">
      {shown && (
        <span inert>
          <BookPages
            data={shown}
            size={size}
            t={bookT}
            eager={false}
            track={noop}
            only={pageOf(kind)}
          />
        </span>
      )}
    </span>
  );
});

/**
 * Which cards are near the viewport: only those draw their thumbnail, and never more than MAX_THUMBS at once. Where IntersectionObserver does not
 * exist (a test) the first MAX_THUMBS are drawn.
 */
function useDrawn(ids: string[]) {
  const root = useRef<HTMLDivElement>(null);
  const has = typeof IntersectionObserver !== "undefined";
  const [near, setNear] = useState<ReadonlySet<string>>(new Set());
  const key = ids.join("|");
  useEffect(() => {
    const el = root.current;
    if (!has || !el) return;
    const io = new IntersectionObserver(
      (entries) =>
        setNear((prev) => {
          const next = new Set(prev);
          for (const e of entries) {
            const id = (e.target as HTMLElement).dataset.tid ?? "";
            if (e.isIntersecting) next.add(id);
            else next.delete(id);
          }
          return next;
        }),
      { rootMargin: "120px" },
    );
    el.querySelectorAll("[data-tid]").forEach((c) => io.observe(c));
    return () => io.disconnect();
  }, [has, key]);
  const drawn = new Set((has ? ids.filter((id) => near.has(id)) : ids).slice(0, MAX_THUMBS));
  return { root, drawn };
}

/**
 * The Templates side panel (T-175): the templates of the page kind being edited, shipped and the student's own, each with a thumbnail, Preview,
 * Apply (and Delete for an own one). It is mounted only while open; the list is read then, again when the kind changes (kept for the two newest
 * kinds), and on "Refresh" or when the shell changes `key`; never polled. Preview and Apply are the shell's (they need the editor and the stage).
 * T-176 adds "Save this page as a template" next to Refresh (`.ed-tpl-actions`) and one more entry in `Lists` is all its new template needs.
 */
export function TemplatesPanel({
  bookId,
  kind,
  data,
  doc,
  size,
  bookT,
  previewId,
  busy,
  announce,
  onPreview,
  onApply,
  onDeleted,
  children,
}: {
  bookId: string;
  kind: SectionId;
  /** The book's data; the thumbnails draw `doc` with its page of `kind` replaced by the template's (so `data.layout` is not used). */
  data: BookData;
  doc: LayoutDoc;
  size: PageSize;
  bookT: TFunction;
  previewId: string | null;
  /** An apply is running: the buttons wait. */
  busy: boolean;
  announce: (text: string) => void;
  onPreview: (card: TemplateCard) => void;
  onApply: (card: TemplateCard) => void;
  onDeleted: (card: TemplateCard) => void;
  /** The preview banner and the apply error: inside the panel, so that Shift+Tab from the list reaches them in a few steps. */
  children?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  const errorText = useTemplateErrorText();
  const [lists, setLists] = useState<Lists>({});
  const [failure, setFailure] = useState<{
    kind: string;
    error: TemplateError;
    seconds?: number;
  } | null>(null);
  const [nonce, setNonce] = useState(0);
  const [cursor, setCursor] = useState(0);
  const [deleting, setDeleting] = useState<TemplateCard | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const radios = useRef<(HTMLButtonElement | null)[]>([]);
  const aside = useRef<HTMLElement>(null);

  const have = lists[kind] !== undefined;
  useEffect(() => aside.current?.focus(), []);
  useEffect(() => {
    if (have) return;
    let live = true;
    listPageTemplates(bookId, kind).then(
      (r) => {
        if (!live) return;
        setLists((l) => remember(l, kind, r.items));
        setFailure(null);
      },
      (e: unknown) =>
        live &&
        setFailure({
          kind,
          error: templateError(e),
          seconds: e instanceof ApiError ? e.retryAfter : undefined,
        }),
    );
    return () => void (live = false);
  }, [bookId, kind, have, nonce]);
  const refresh = useCallback(() => {
    setFailure(null);
    setLists({});
    setNonce((n) => n + 1);
  }, []);

  const items = lists[kind];
  const groups = useMemo(
    () => (items ? groupTemplates(items, i18n.language) : null),
    [items, i18n.language],
  );
  const cards = useMemo(() => (groups ? allCards(groups) : []), [groups]);
  const { root, drawn } = useDrawn(cards.map(cardKey));

  const move = (e: KeyboardEvent, from: number) => {
    const n = cards.length;
    const to =
      e.key === "ArrowDown" || e.key === "ArrowRight"
        ? Math.min(from + 1, n - 1)
        : e.key === "ArrowUp" || e.key === "ArrowLeft"
          ? Math.max(from - 1, 0)
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? n - 1
              : from;
    if (!["ArrowDown", "ArrowRight", "ArrowUp", "ArrowLeft", "Home", "End"].includes(e.key)) return;
    e.preventDefault();
    setCursor(to);
    radios.current[to]?.focus();
  };

  const confirmDelete = async () => {
    if (!deleting || deleteBusy) return;
    const card = deleting;
    setDeleteBusy(true);
    setDeleteError("");
    try {
      await deleteOwnTemplate(card.id);
    } catch (e) {
      setDeleteBusy(false);
      if (templateError(e) === "gone") {
        // already gone (another tab): the card goes with the next list
        setDeleting(null);
        onDeleted(card);
        refresh();
        announce(t("editor.templates.err.gone"));
      } else
        setDeleteError(
          errorText(templateError(e), e instanceof ApiError ? e.retryAfter : undefined),
        );
      return;
    }
    setDeleteBusy(false);
    setDeleting(null);
    setLists((l) => ({ ...l, [kind]: (l[kind] ?? []).filter((x) => x.id !== card.id) }));
    setCursor(0);
    onDeleted(card);
    announce(t("editor.templates.deleted", { name: card.name }));
    setTimeout(() => aside.current?.focus(), 0); // the card that had the focus is gone
  };

  let n = 0;
  const group = (title: string, list: TemplateCard[]) => (
    <div role="group" aria-label={title} className="ed-tpl-group">
      <h3>{title}</h3>
      {list.length === 0 && <p className="ed-hint">{t("editor.templates.empty")}</p>}
      {list.map((c) => {
        const i = n++;
        const label = t("editor.templates.cardLabel", {
          name: c.name,
          items: t("editor.templates.items", { count: c.items }),
          source: t(`editor.templates.source.${c.source}`),
        });
        return (
          <div className="ed-tpl-card" key={cardKey(c)} data-tid={cardKey(c)}>
            <button
              type="button"
              role="radio"
              aria-checked={cardKey(c) === previewId}
              aria-label={label}
              tabIndex={i === cursor ? 0 : -1}
              ref={(el) => void (radios.current[i] = el)}
              className="ed-tpl-pick"
              onFocus={() => setCursor(i)}
              onKeyDown={(e) => move(e, i)}
              onClick={() => onPreview(c)}
            >
              <Thumb
                data={data}
                doc={doc}
                kind={kind}
                card={c}
                size={size}
                bookT={bookT}
                drawn={drawn.has(cardKey(c))}
              />
              <span className="ed-tpl-name">{c.name}</span>
              <span className="ed-hint">
                {t("editor.templates.items", { count: c.items })}
                {c.source === "own" && <> · {t("editor.templates.noImages")}</>}
              </span>
            </button>
            <span className="ed-tpl-buttons">
              <Button disabled={busy} onClick={() => onApply(c)}>
                {t("editor.templates.apply")}
              </Button>
              {c.source === "own" && (
                <Button
                  variant="danger"
                  disabled={busy}
                  onClick={() => {
                    setDeleteError("");
                    setDeleting(c);
                  }}
                >
                  {t("editor.templates.delete")}
                </Button>
              )}
            </span>
          </div>
        );
      })}
    </div>
  );

  const failed = failure?.kind === kind ? failure : null;
  const title = t(`editor.templates.title.${kind}`);
  return (
    <aside className="ed-panel ed-templates" aria-label={title} ref={aside} tabIndex={-1}>
      <h2>{title}</h2>
      {kind === "notes" && <p className="ed-hint">{t("editor.templates.notesShared")}</p>}
      {children}
      <div className="ed-tpl-actions">
        <Button onClick={refresh} disabled={!items && !failed}>
          {t("editor.templates.refresh")}
        </Button>
      </div>
      {!items && !failed && <p role="status">{t("editor.templates.loading")}</p>}
      {failed && (
        <Banner variant="error" action={<Button onClick={refresh}>{t("auth.retry")}</Button>}>
          {errorText(failed.error, failed.seconds)}
        </Banner>
      )}
      {groups && (
        <div
          role="radiogroup"
          aria-label={t("editor.templates.list")}
          className="ed-tpl-list"
          ref={root}
        >
          {group(t("editor.templates.shipped"), groups.shipped)}
          {group(t("editor.templates.own"), groups.own)}
        </div>
      )}
      {deleting && (
        <Dialog
          title={t("editor.templates.deleteTitle")}
          confirmLabel={t("editor.templates.delete")}
          cancelLabel={t("editor.templates.cancel")}
          danger
          busy={deleteBusy}
          onConfirm={() => void confirmDelete()}
          onCancel={() => !deleteBusy && setDeleting(null)}
        >
          <p>{t("editor.templates.deleteBody", { name: deleting.name })}</p>
          {deleteError && (
            <p role="alert" className="error">
              {deleteError}
            </p>
          )}
        </Dialog>
      )}
    </aside>
  );
}

/** The bar over the list while a template is previewed: what it is, how many items would be left out, and the two ways out. */
export function TemplateBanner({
  name,
  what,
  leaving,
  busy,
  onApply,
  onBack,
}: {
  name: string;
  what: string;
  leaving: number;
  busy: boolean;
  onApply: () => void;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Banner
      variant="warning"
      action={
        <>
          <Button variant="primary" busy={busy} onClick={onApply}>
            {t("editor.templates.applyTo", { what })}
          </Button>
          <Button disabled={busy} onClick={onBack}>
            {t("editor.templates.back")}
          </Button>
        </>
      }
    >
      {t("editor.templates.previewing", { name, what })}
      {leaving > 0 && <> {t("editor.templates.willLeaveOut", { count: leaving })}</>}
    </Banner>
  );
}
