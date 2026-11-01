import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { BookData } from "../../api/book";
import { errorText } from "../../auth/errors";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { Skeleton } from "../../components/ui/Skeleton";
import { listPages } from "../BookPages";
import { pageBox } from "../layout/geometry";
import { getTemplate } from "../templates";
import { pageName, targetName } from "./names";
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
  const spec = getTemplate(data.yearbook.template_id).layout;
  const bookT = useMemo(
    () => i18n.getFixedT(data.yearbook.language),
    [i18n, data.yearbook.language],
  ); // the book's own language, not the app's
  const pages = useMemo(() => listPages(data), [data]);
  const [pageIdx, setPageIdx] = useState(0);
  const index = Math.min(pageIdx, pages.length - 1);
  const page = pages[index]!;
  const kind = page.section;
  const shown = useMemo(() => ({ ...data, layout: ed.doc }), [data, ed.doc]);
  const [said, say] = useState("");
  const [asking, setAsking] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [resetError, setResetError] = useState<unknown>(null);
  const [previewFailed, setPreviewFailed] = useState(false);
  const readOnly = ed.saveState === "conflict";

  const pb = useMemo(() => pageBox(size), [size]);
  // the stage reports where it measured the template's own elements, so the panel shows the numbers of what is drawn
  const [measuredTargets, setTargets] = useState<Target[]>([]);
  const targets = measuredTargets.length ? measuredTargets : targetsOf(ed.doc, kind, spec, pb);
  const selected = targets.find((x) => x.id === ed.selection[0]);

  const { select, undo, redo } = ed;
  const pick = (i: number) => {
    if (i === index) return;
    setPageIdx(i);
    select([]);
    say(t("editor.pageOf", { n: i + 1, total: pages.length, name: pageName(t, pages[i]!) }));
  };
  const choose = useCallback(
    (ids: readonly string[]) => {
      select(ids);
      const x = targets.find((y) => y.id === ids[0]);
      say(x ? t("editor.selected", { name: targetName(t, x) }) : t("editor.cleared"));
    },
    [select, targets, t, say],
  );

  // undo and redo from the keyboard (not while a text area has the keys, T-161)
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const el = e.target instanceof HTMLElement ? e.target : null;
      if (el?.tagName === "TEXTAREA" || el?.isContentEditable) return;
      const k = e.key.toLowerCase();
      const mod = e.ctrlKey || e.metaKey;
      if (e.key === "Escape" && !asking) return choose([]);
      if (!mod || readOnly) return;
      if (k === "z" && !e.shiftKey) undo();
      else if ((k === "z" && e.shiftKey) || (k === "y" && e.ctrlKey)) redo();
      else return;
      e.preventDefault();
    };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [undo, redo, readOnly, asking, choose]);

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
      <div className="ed-grid">
        <PageStrip
          data={shown}
          size={size}
          bookT={bookT}
          pages={pages}
          current={index}
          onPick={pick}
        />
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
        />
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
        />
      </div>
      <div className="visually-hidden" role="status" aria-live="polite">
        {said}
      </div>
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
