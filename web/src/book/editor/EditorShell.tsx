import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { BookData } from "../../api/book";
import { restoreLayoutVersion, type LayoutVersion } from "../../api/bookLayout";
import { deleteMedia, uploadMedia, type Media } from "../../api/yearbooks";
import { getAsset } from "../assets";
import { errorText } from "../../auth/errors";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { Skeleton } from "../../components/ui/Skeleton";
import { useMediaQuery } from "../../components/useMediaQuery";
import { resolveDesign } from "../design";
import { useFitReport } from "../fit";
import { listPages } from "../BookPages";
import { pageBox } from "../layout/geometry";
import { MissingImage } from "../layout/runtime";
import { getTemplate } from "../templates";
import {
  addItem,
  duplicateItem,
  mediaUses,
  newItemId,
  removeBlankText,
  removeProblems,
  type LayoutItem,
} from "./document";
import { HistoryPanel, PreviewBanner, useErrorText } from "./HistoryPanel";
import { pageName, targetName } from "./names";
import { missingCount, restoreVersion } from "./versions";
import { HelpSheet } from "./gestures/HelpSheet";
import { deleteDesignImage } from "./palette/deleteImage";
import { DesignImages } from "./palette/DesignImages";
import { copyShift } from "./palette/defaults";
import { ALL_KINDS, Palette, type Tab } from "./palette/Palette";
import { Uploads } from "./palette/uploads";
import { useDesignImages } from "./palette/useDesignImages";
import { PageStrip } from "./PageStrip";
import { PropertyPanel } from "./PropertyPanel";
import { SaveStatus } from "./SaveStatus";
import { Stage } from "./Stage";
import { targetsOf, type Target } from "./targets";
import { useLayoutEditor } from "./useLayoutEditor";
import "../../styles/editor.css";

/**
 * The free-layout editor of one book (epic E09 stage 2, T-159): toolbar, page strip, stage and property panel around the T-158 store. Mounted only on a
 * wide screen (the route shows a message below 768 px). Drag, resize and rotate gestures are T-160, typing text T-161.
 */
export function EditorShell({
  bookId,
  data,
  safeMm,
}: {
  bookId: string;
  data: BookData;
  safeMm: number;
}) {
  const { t, i18n } = useTranslation();
  const ed = useLayoutEditor(bookId);
  const size = data.yearbook.page_size;
  const tpl = getTemplate(data.yearbook.template_id);
  const spec = tpl.layout;
  const scope = resolveDesign(data.design, tpl.design);
  const bookT = useMemo(
    () => i18n.getFixedT(data.yearbook.language),
    [i18n, data.yearbook.language],
  ); // the book's own language, not the app's
  const pages = useMemo(() => listPages(data), [data]);
  const [pageIdx, setPageIdx] = useState(0);
  const index = Math.min(pageIdx, pages.length - 1);
  const page = pages[index]!;
  const kind = page.section;
  // history (T-165): the panel replaces the property panel; a previewed version is drawn read-only instead of the edited document
  const [historyOpen, setHistoryOpen] = useState(false);
  const [viewed, setViewed] = useState<LayoutVersion | null>(null);
  const [historyKey, setHistoryKey] = useState(0);
  const [confirmRestore, setConfirmRestore] = useState(false);
  const [restoring, setRestoring] = useState(false);
  const [restoreError, setRestoreError] = useState("");
  const [toast, setToast] = useState("");
  const restoreOnce = useRef(false);
  const historyButton = useRef<HTMLButtonElement>(null);
  const historyText = useErrorText();
  const shown = useMemo(
    () => ({ ...data, layout: viewed ? viewed.layout : ed.doc }),
    [data, ed.doc, viewed],
  );
  const [said, say] = useState("");
  const [asking, setAsking] = useState(false);
  const [help, setHelp] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [resetError, setResetError] = useState<unknown>(null);
  const [previewFailed, setPreviewFailed] = useState(false);
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState<{ id: string; all: boolean } | null>(null);
  const [tab, setTab] = useState<Tab>("sticker");
  // The palette is a fourth column, so it is open at first only on a screen wide enough to leave the stage its room; the toolbar button toggles it.
  const wide = useMediaQuery("(min-width: 1700px)");
  const [chosen, setPalette] = useState<boolean | null>(null);
  const paletteOpen = chosen ?? wide;
  const [deleting, setDeleting] = useState<{ m: Media; uses: string[]; history: boolean } | null>(
    null,
  );
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const { states: fits, Provider: Fit } = useFitReport();
  const readOnly = ed.saveState === "conflict" || viewed !== null;

  const pb = useMemo(() => pageBox(size), [size]);
  // the stage reports where it measured the template's own elements, so the panel shows the numbers of what is drawn
  const [measuredTargets, setTargets] = useState<Target[]>([]);
  const targets = measuredTargets.length
    ? measuredTargets
    : targetsOf(shown.layout ?? ed.doc, kind, spec, pb);
  const selected = targets.find((x) => x.id === ed.selection[0]);

  const { select, undo, redo, apply, begin, preview, commit, cancel } = ed;
  const cut =
    selected?.item?.t === "text" && fits[`layout:${kind}:${selected.id}`]?.state === "overflow";
  const store = useMemo(
    () => ({ begin, preview, commit, cancel, apply, select }),
    [begin, preview, commit, cancel, apply, select],
  );
  const images = useDesignImages(bookId);
  const uploads = useMemo(() => new Uploads({ yearbookId: bookId, upload: uploadMedia }), [bookId]);
  const { prepend } = images;
  useEffect(
    () =>
      uploads.bind({
        onDone: (m) => {
          prepend(m);
          say(t("editor.upload.finished"));
        },
      }),
    [uploads, prepend, t, say],
  );
  const missing = useMemo(
    () => (
      <div className="ed-missing" role="img" aria-label={t("editor.image.missing")}>
        {t("editor.image.missing")}
      </div>
    ),
    [t],
  );
  // every page type of the template, also one the book has no page of yet (no notes), so "use on all page types" covers them all
  const kinds = useMemo(
    () => [...new Set([...ALL_KINDS, ...pages.map((p) => p.section)])],
    [pages],
  );
  const stripData = useDeferredValue(shown); // the thumbnails catch up after a drag instead of redrawing on every pointer move
  const known = useRef(targets);
  useEffect(() => {
    known.current = targets;
  });
  const edit = useCallback((id: string) => setEditing({ id, all: false }), [setEditing]);
  /** Edit mode ends (T-161). A text left with nothing but white space is removed, as one step that undo brings back. */
  const editDone = useCallback(
    (id: string, to: { escape: true } | { blurred: EventTarget | null }) => {
      // the focus moved into the property panel (a font, a colour): the box stays selected and open for the next click into it
      if ("blurred" in to && to.blurred instanceof Element && to.blurred.closest(".ed-panel"))
        return;
      setEditing((cur) => (cur?.id === id ? null : cur));
      let removed = false;
      apply((doc) => {
        const r = removeBlankText(kind, id)(doc);
        removed = r.doc !== doc;
        return r;
      });
      if (removed) select([]);
      if ("escape" in to)
        setTimeout(
          () =>
            document
              .querySelector<HTMLElement>(`[data-target="${CSS.escape(id)}"] .ed-hit`)
              ?.focus(),
          0,
        );
    },
    [apply, kind, select, setEditing],
  );
  const addText = useCallback(() => {
    const text: LayoutItem = {
      id: newItemId(),
      t: "text",
      text: t("editor.newText"),
      x: 200,
      w: 600,
      h: 200,
      cy: 500,
      r: 0,
      font: "body",
      size: Math.round(((spec?.textSize.min ?? 8) + (spec?.textSize.max ?? 96)) / 2),
      color: Object.keys(spec?.colors ?? {})[0] ?? "ink",
      align: "left",
      bold: false,
    };
    const why = apply(addItem(kind, text));
    setNotice(why ? t(`editor.refused.${why}`, { defaultValue: t("editor.refused.other") }) : "");
    if (why) return;
    select([text.id]);
    setEditing({ id: text.id, all: true });
    say(t("editor.textAdded"));
  }, [apply, kind, select, spec, t, say, setEditing, setNotice]);
  /** Adds a palette item (sticker, shape, frame, image) on top of the edited page, selected and focused so the keys of T-160 move it. */
  const addFromPalette = useCallback(
    (item: LayoutItem, what: string) => {
      const why = apply(addItem(kind, item));
      setNotice(why ? t(`editor.refused.${why}`, { defaultValue: t("editor.refused.other") }) : "");
      if (why) return;
      select([item.id]);
      setEditing(null);
      say(what);
      setTimeout(
        () =>
          document
            .querySelector<HTMLElement>(`[data-target="${CSS.escape(item.id)}"] .ed-hit`)
            ?.focus(),
        0,
      );
    },
    [apply, kind, select, t, say, setEditing, setNotice],
  );
  const duplicate = useCallback(() => {
    const src = selected?.item;
    if (!src) return;
    const id = newItemId();
    const { dx, dcy } = copyShift(pb);
    const why = apply(duplicateItem(kind, src.id, id, dx, dcy));
    setNotice(why ? t(`editor.refused.${why}`, { defaultValue: t("editor.refused.other") }) : "");
    if (why) return;
    select([id]);
    say(t("editor.duplicated"));
  }, [apply, kind, pb, select, selected, t, say, setNotice]);
  const askDelete = (m: Media) => {
    setDeleteError("");
    setDeleting({
      m,
      uses: mediaUses(ed.doc, m.id),
      history: ed.usesImageInHistory(m.id),
    });
  };
  const confirmDelete = async () => {
    if (!deleting) return;
    setDeleteBusy(true);
    setDeleteError("");
    const r = await deleteDesignImage(deleting.m.id, {
      doc: () => ed.doc,
      apply: ed.apply,
      settle: ed.settle,
      clearHistory: ed.clearHistory,
      usesInHistory: ed.usesImageInHistory,
      deleteMedia,
    });
    setDeleteBusy(false);
    if (r.ok) {
      images.drop(deleting.m.id);
      setDeleting(null);
      select([]);
      say(t("editor.deleteImage.done"));
    } else
      setDeleteError(
        r.stage === "save"
          ? t("editor.deleteImage.saveFailed")
          : `${t("editor.deleteImage.deleteFailed")} ${errorText(t, r.error)}`,
      );
  };
  const pick = (i: number) => {
    if (i === index) return;
    setPageIdx(i);
    setEditing(null);
    setNotice("");
    select([]);
    say(t("editor.pageOf", { n: i + 1, total: pages.length, name: pageName(t, pages[i]!) }));
  };
  const choose = useCallback(
    (ids: readonly string[]) => {
      select(ids);
      setNotice("");
      setEditing((cur) => (cur && ids[0] !== cur.id ? null : cur));
      const x = known.current.find((y) => y.id === ids[0]);
      say(x ? t("editor.selected", { name: targetName(t, x) }) : t("editor.cleared"));
    },
    [select, t, say, setEditing, setNotice],
  );

  // --- version history (T-165) ---
  const closeHistory = useCallback(() => {
    setViewed(null);
    setRestoreError("");
    setHistoryOpen(false);
    setTimeout(() => historyButton.current?.focus(), 0);
  }, [setViewed, setRestoreError, setHistoryOpen]);
  const backToCurrent = useCallback(() => {
    setViewed(null);
    setRestoreError("");
    say(t("editor.history.backDone"));
  }, [t, say, setViewed, setRestoreError]);
  const showPreview = useCallback(
    (v: LayoutVersion) => {
      select([]);
      setEditing(null);
      setNotice("");
      setRestoreError("");
      setViewed(v);
      say(t("editor.history.previewingShort"));
    },
    [select, t, say, setEditing, setNotice, setViewed, setRestoreError],
  );
  // a design image list that is complete lets the banner count the images that are gone; a partial one counts none (the hint is only a hint)
  const knownMedia = useMemo(
    () =>
      images.status === "ready" && !images.more ? new Set(images.items.map((m) => m.id)) : null,
    [images.status, images.more, images.items],
  );
  const dropping = viewed
    ? missingCount(
        viewed.layout,
        (id) => !!getAsset(id),
        knownMedia && ((id) => knownMedia.has(id)),
      )
    : 0;
  const doRestore = async () => {
    if (!viewed || restoreOnce.current) return; // a double click sends one request
    restoreOnce.current = true;
    setRestoring(true);
    setRestoreError("");
    const r = await restoreVersion(viewed.id, {
      settle: ed.settle,
      version: ed.currentVersion,
      restore: (id, base) => restoreLayoutVersion(bookId, id, base),
      adopt: ed.adopt,
      enterConflict: ed.enterConflict,
    });
    restoreOnce.current = false;
    setRestoring(false);
    setConfirmRestore(false);
    if (r.ok) {
      const msg =
        t("editor.history.restored") +
        (r.dropped > 0
          ? ` ${t(r.dropped === 1 ? "editor.history.droppedMsgOne" : "editor.history.droppedMsg", { count: r.dropped })}`
          : "");
      setViewed(null);
      setHistoryKey((k) => k + 1); // the replaced document is a new "Before restore" row
      setToast(msg);
      say(msg);
      return;
    }
    setRestoreError(historyText(r.error, r.retryAfter));
    if (r.error === "conflict") setViewed(null); // the conflict banner takes over with the stored document
    if (r.error === "gone") setHistoryKey((k) => k + 1);
  };

  // undo and redo from the keyboard (not while a text area has the keys, T-161)
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const el = e.target instanceof HTMLElement ? e.target : null;
      if (el?.tagName === "TEXTAREA" || el?.isContentEditable) return;
      if (e.key === "?" && el?.tagName !== "INPUT" && el?.tagName !== "SELECT" && !asking) {
        e.preventDefault();
        return setHelp((h) => !h);
      }
      if (e.key === "Escape" && help) return setHelp(false);
      const k = e.key.toLowerCase();
      const mod = e.ctrlKey || e.metaKey;
      if (e.key === "Escape" && historyOpen && !asking && !confirmRestore && !deleting) {
        e.preventDefault();
        return viewed ? backToCurrent() : closeHistory();
      }
      if (e.key === "Escape" && !asking) return choose([]);
      if (!mod || readOnly) return;
      if (k === "z" && !e.shiftKey) undo();
      else if ((k === "z" && e.shiftKey) || (k === "y" && e.ctrlKey)) redo();
      else if (k === "d" && !e.shiftKey && !e.altKey && el?.tagName !== "INPUT") duplicate();
      else return;
      e.preventDefault();
    };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [
    undo,
    redo,
    readOnly,
    asking,
    help,
    choose,
    duplicate,
    historyOpen,
    viewed,
    confirmRestore,
    deleting,
    backToCurrent,
    closeHistory,
  ]);

  if (ed.saveState === "gone")
    return (
      <div role="alert">
        <h2>{t("editor.goneTitle")}</h2>
        <p>{t("editor.goneBody")}</p>
        <Link to="/yearbooks">{t("editor.goneLink")}</Link>
      </div>
    );
  if (ed.loaded === "failed")
    return (
      <Banner
        variant="error"
        action={<Button onClick={() => void ed.reload()}>{t("auth.retry")}</Button>}
      >
        {t("editor.loadFailed")}
      </Banner>
    );
  if (ed.loaded === "loading")
    return (
      <div role="status" aria-live="polite">
        <p>{t("editor.loading")}</p>
        <Skeleton width="100%" height={240} />
      </div>
    );

  return (
    <div className="ed">
      <div className="ed-toolbar" role="toolbar" aria-label={t("editor.toolbar")}>
        <button type="button" onClick={undo} disabled={readOnly || !ed.canUndo}>
          {t("editor.undo")}
        </button>
        <button type="button" onClick={redo} disabled={readOnly || !ed.canRedo}>
          {t("editor.redo")}
        </button>
        <button type="button" disabled={readOnly || !spec} onClick={addText}>
          {t("editor.addText")}
        </button>
        <button
          type="button"
          aria-expanded={paletteOpen}
          aria-controls="ed-palette"
          onClick={() => setPalette(!paletteOpen)}
        >
          {t("editor.palette.toggle")}
        </button>
        <button
          type="button"
          ref={historyButton}
          aria-expanded={historyOpen}
          onClick={() => (historyOpen ? closeHistory() : setHistoryOpen(true))}
        >
          <svg
            aria-hidden="true"
            width="14"
            height="14"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
          >
            <circle cx="8" cy="8" r="6.2" />
            <path d="M8 4.5V8l2.4 1.6" />
          </svg>{" "}
          {t("editor.history.button")}
        </button>
        <SaveStatus state={ed.saveState} dirty={ed.dirty} />
        <button
          type="button"
          disabled={readOnly}
          onClick={() => {
            setResetError(null);
            setAsking(true);
          }}
        >
          {t("editor.resetAll")}
        </button>
        <button type="button" aria-expanded={help} onClick={() => setHelp((h) => !h)}>
          {t("editor.helpButton")}
        </button>
        <a
          href={`/yearbooks/${bookId}/book`}
          target="_blank"
          rel="noopener"
          onClick={(e) => {
            // The tab is opened by the click (a popup blocker allows that) and sent to the book once everything is saved, the save in flight and the one
            // queued behind it included; if saving fails, the tab is closed and the editor says so.
            if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey) return void ed.flush();
            e.preventDefault();
            setPreviewFailed(false);
            const tab = window.open("about:blank", "_blank");
            if (tab) tab.opener = null;
            const url = `/yearbooks/${bookId}/book`;
            void ed.settle().then((saved) => {
              if (!saved) {
                tab?.close();
                setPreviewFailed(true);
              } else if (tab) tab.location.href = url;
              else window.open(url, "_blank", "noopener");
            });
          }}
        >
          {t("editor.preview")}
        </a>
      </div>
      {help && <HelpSheet onClose={() => setHelp(false)} />}
      {notice && (
        <p role="alert" className="error">
          {notice}
        </p>
      )}
      {previewFailed && (
        <p role="alert" className="error">
          {t("editor.previewFailed")}
        </p>
      )}
      {ed.saveState === "conflict" && (
        <Banner
          variant="warning"
          action={
            <>
              <Button onClick={() => void ed.useTheirs()}>{t("editor.useSaved")}</Button>
              <Button onClick={() => void ed.keepMine()}>{t("editor.keepMine")}</Button>
            </>
          }
        >
          {t("editor.conflict")}
        </Banner>
      )}
      {ed.saveState === "invalid" && (
        <Banner variant="error">
          {t("editor.invalid")}
          {removeProblems(ed.problems.map((p) => p.path))(ed.doc).doc !== ed.doc && (
            <Button
              onClick={() => {
                ed.apply(removeProblems(ed.problems.map((p) => p.path)));
                say(t("editor.invalidRemoved"));
              }}
            >
              {t("editor.invalidRemove")}
            </Button>
          )}
          {ed.problems.length > 0 && (
            <ul>
              {ed.problems.map((p) => (
                <li key={`${p.path}${p.code}`}>
                  {p.path}: {p.code}
                </li>
              ))}
            </ul>
          )}
        </Banner>
      )}
      {toast && (
        <Banner
          variant="warning"
          action={<Button onClick={() => setToast("")}>{t("editor.history.dismiss")}</Button>}
        >
          {toast}
        </Banner>
      )}
      <div
        className={`ed-grid${paletteOpen ? " has-palette" : ""}${viewed ? " is-previewing" : ""}`}
      >
        <PageStrip
          data={stripData}
          size={size}
          bookT={bookT}
          pages={pages}
          current={index}
          onPick={pick}
        />
        <MissingImage.Provider value={missing}>
          <Stage
            data={shown}
            size={size}
            bookT={bookT}
            page={page}
            kind={kind}
            spec={spec}
            selection={ed.selection}
            onSelect={choose}
            onTargets={setTargets}
            readOnly={readOnly}
            safeMm={safeMm}
            store={store}
            announce={say}
            Fit={Fit}
            editing={editing}
            onEdit={edit}
            onEditDone={editDone}
            cut={cut}
          />
        </MissingImage.Provider>
        {historyOpen ? (
          <HistoryPanel
            bookId={bookId}
            previewId={viewed?.id ?? null}
            refreshKey={historyKey}
            onPreview={showPreview}
          >
            {viewed && (
              <PreviewBanner
                version={viewed}
                dropping={dropping}
                busy={restoring}
                onRestore={() => setConfirmRestore(true)}
                onBack={backToCurrent}
              />
            )}
            {restoreError && (
              <p role="alert" className="error">
                {restoreError}
              </p>
            )}
          </HistoryPanel>
        ) : (
          <PropertyPanel
            doc={ed.doc}
            kind={kind}
            page={page}
            pb={pb}
            selected={selected}
            apply={ed.apply}
            select={choose}
            readOnly={readOnly}
            announce={say}
            spec={spec}
            scope={scope}
            store={store}
            cut={cut}
            onAddText={addText}
            onDuplicate={duplicate}
            onMoreBackgrounds={() => {
              setTab("background");
              setPalette(true);
            }}
            pickImage={(onPick) => (
              <DesignImages
                images={images}
                uploads={uploads}
                readOnly={readOnly}
                pickLabel={(n, total) => t("editor.image.pickN", { n, total })}
                onPick={onPick}
              />
            )}
          />
        )}
        {paletteOpen && (
          <Palette
            tab={tab}
            onTab={setTab}
            kind={kind}
            kinds={kinds}
            doc={ed.doc}
            apply={ed.apply}
            readOnly={readOnly}
            announce={say}
            spec={spec}
            scope={scope}
            images={images}
            uploads={uploads}
            onAdd={addFromPalette}
            onAskDelete={askDelete}
          />
        )}
      </div>
      <div className="visually-hidden" role="status" aria-live="polite">
        {said}
      </div>
      {confirmRestore && (
        <Dialog
          title={t("editor.history.confirmTitle")}
          confirmLabel={t("editor.history.restore")}
          cancelLabel={t("editor.history.cancel")}
          busy={restoring}
          onConfirm={() => void doRestore()}
          onCancel={() => !restoring && setConfirmRestore(false)}
        >
          <p>{t("editor.history.confirmBody")}</p>
        </Dialog>
      )}
      {asking && (
        <Dialog
          title={t("editor.resetTitle")}
          confirmLabel={t("editor.resetConfirm")}
          cancelLabel={t("editor.resetCancel")}
          danger
          busy={resetting}
          onConfirm={() => {
            setResetting(true);
            ed.reset().then(
              () => {
                setResetting(false);
                setAsking(false);
                say(t("editor.resetDone"));
              },
              (e: unknown) => {
                setResetting(false);
                setResetError(e);
              },
            );
          }}
          onCancel={() => setAsking(false)}
        >
          <p>{t("editor.resetBody")}</p>
          {resetError !== null && (
            <p role="alert" className="error">
              {errorText(t, resetError)}
            </p>
          )}
        </Dialog>
      )}
      {deleting && (
        <Dialog
          title={t("editor.deleteImage.title")}
          confirmLabel={t("editor.deleteImage.confirm")}
          cancelLabel={t("editor.deleteImage.cancel")}
          danger
          busy={deleteBusy}
          onConfirm={() => void confirmDelete()}
          onCancel={() => !deleteBusy && setDeleting(null)}
        >
          <p>
            {deleting.uses.length > 0
              ? t("editor.deleteImage.inUse", {
                  count: deleting.uses.length,
                  where: deleting.uses.map((k) => t(`editor.palette.kind.${k}`)).join(", "),
                })
              : deleting.history
                ? t("editor.deleteImage.onlyHistory")
                : t("editor.deleteImage.plain")}
          </p>
          {(deleting.uses.length > 0 || deleting.history) && (
            <p>{t("editor.deleteImage.history")}</p>
          )}
          {deleteError && (
            <p role="alert" className="error">
              {deleteError}
            </p>
          )}
        </Dialog>
      )}
      {ed.leaveGuard.pending && (
        <Dialog
          title={t("editor.leaveTitle")}
          confirmLabel={t("editor.leaveConfirm")}
          cancelLabel={t("editor.leaveCancel")}
          danger
          onConfirm={ed.leaveGuard.leave}
          onCancel={ed.leaveGuard.stay}
        >
          <p>{t("editor.leaveBody")}</p>
        </Dialog>
      )}
    </div>
  );
}
