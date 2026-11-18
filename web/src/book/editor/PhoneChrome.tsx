import {
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react";
import { useTranslation } from "react-i18next";
import type { OnePage } from "../BookPages";
import { Button } from "../../components/ui/Button";
import { pageName, targetName } from "./names";
import { SaveStatus } from "./SaveStatus";
import type { SaveState } from "./store";
import type { Target } from "./targets";

/** What the on-screen keyboard takes from the screen: the layout viewport minus the visual viewport (iOS Safari and Android Chrome shrink only the latter). */
export function useVisualViewport(): { inset: number; height: number } {
  const [v, setV] = useState({ inset: 0, height: 0 });
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const update = () =>
      setV({
        inset: Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop)),
        height: Math.round(vv.height),
      });
    update();
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
    };
  }, []);
  return v;
}

export type SheetName = "edit" | "layers" | "add" | "pages";

/** The top bar of the phone editor: undo and redo, the autosave status, History, Templates, Print and Done. Every button is at least 44 px high (CSS). */
export function PhoneBar({
  readOnly,
  canUndo,
  canRedo,
  onUndo,
  onRedo,
  saveState,
  dirty,
  hasTemplates,
  historyRef,
  templatesRef,
  historyOpen,
  templatesOpen,
  onHistory,
  onTemplates,
  onPrint,
  printHref,
  finishing,
  onDone,
}: {
  readOnly: boolean;
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  saveState: SaveState;
  dirty: boolean;
  hasTemplates: boolean;
  historyRef: Ref<HTMLButtonElement>;
  templatesRef: Ref<HTMLButtonElement>;
  historyOpen: boolean;
  templatesOpen: boolean;
  onHistory: () => void;
  onTemplates: () => void;
  onPrint: (e: React.MouseEvent<HTMLAnchorElement>) => void;
  printHref: string;
  finishing: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="ed-pbar">
      <div className="ed-prow" role="toolbar" aria-label={t("editor.toolbar")}>
        <button type="button" onClick={onUndo} disabled={readOnly || !canUndo}>
          {t("editor.undo")}
        </button>
        <button type="button" onClick={onRedo} disabled={readOnly || !canRedo}>
          {t("editor.redo")}
        </button>
        <SaveStatus state={saveState} dirty={dirty} />
        <Button variant="primary" busy={finishing} onClick={onDone}>
          {t("editor.done")}
        </Button>
      </div>
      <div className="ed-prow" role="group" aria-label={t("editor.flow")}>
        <button type="button" ref={historyRef} aria-expanded={historyOpen} onClick={onHistory}>
          {t("editor.history.button")}
        </button>
        {hasTemplates && (
          <button
            type="button"
            ref={templatesRef}
            aria-expanded={templatesOpen}
            onClick={onTemplates}
          >
            {t("editor.templates.button")}
          </button>
        )}
        <a className="ed-plink" href={printHref} target="_blank" rel="noopener" onClick={onPrint}>
          {t("editor.print")}
        </a>
      </div>
    </div>
  );
}

/**
 * The bottom of the phone editor (T-181): the sheet (a labelled region: the properties of the selection, the list of the page's items, the palette or the
 * page list), under it the pager and the three buttons that open the sheet. It sits on the visual viewport, so the on-screen keyboard never covers it.
 */
export function PhoneBottom({
  sheet,
  onSheet,
  index,
  count,
  page,
  onStep,
  inset,
  height,
  edit,
  add,
  targets,
  selectedId,
  onPickTarget,
  pages,
  onPickPage,
  readOnly,
}: {
  sheet: SheetName | null;
  onSheet: (s: SheetName | null) => void;
  index: number;
  count: number;
  page: OnePage;
  onStep: (by: -1 | 1) => void;
  inset: number;
  height: number;
  edit: ReactNode;
  add: ReactNode;
  targets: readonly Target[];
  selectedId: string | undefined;
  onPickTarget: (id: string) => void;
  pages: readonly OnePage[];
  onPickPage: (i: number) => void;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const titleId = useId();
  const style = {
    "--kb": `${inset}px`,
    ...(height ? { "--vvh": `${height}px` } : {}),
  } as CSSProperties;
  const title =
    sheet === "layers"
      ? t("editor.phone.layers")
      : sheet === "add"
        ? t("editor.phone.add")
        : sheet === "pages"
          ? t("editor.phone.pages")
          : t("editor.panel");
  return (
    <div className="ed-bottom" style={style}>
      {sheet && (
        <section className="ed-sheet" aria-labelledby={titleId}>
          <div className="ed-sheet-head">
            <h2 id={titleId}>{title}</h2>
            <button type="button" onClick={() => onSheet(null)}>
              {t("editor.phone.close")}
            </button>
          </div>
          <div className="ed-sheet-body">
            {sheet === "edit" && edit}
            {sheet === "add" && add}
            {sheet === "layers" && (
              <>
                {targets.length === 0 && <p className="ed-hint">{t("editor.phone.noItems")}</p>}
                <ul className="ed-plist">
                  {[...targets].reverse().map((x) => (
                    <li key={x.id}>
                      <button
                        type="button"
                        aria-pressed={x.id === selectedId}
                        onClick={() => onPickTarget(x.id)}
                      >
                        {targetName(t, x)}
                        {x.hidden && <span className="ed-hint"> {t("editor.phone.hidden")}</span>}
                      </button>
                    </li>
                  ))}
                </ul>
              </>
            )}
            {sheet === "pages" && (
              <ul className="ed-plist">
                {pages.map((p, i) => (
                  <li key={`${p.section}:${p.index ?? 0}`}>
                    <button
                      type="button"
                      aria-current={i === index ? "page" : undefined}
                      onClick={() => onPickPage(i)}
                    >
                      {t("editor.pageOf", { n: i + 1, total: count, name: pageName(t, p) })}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
      )}
      <div className="ed-pager" role="group" aria-label={t("editor.phone.pager")}>
        <button type="button" aria-disabled={index === 0} onClick={() => onStep(-1)}>
          {t("editor.prevPage")}
        </button>
        <button
          type="button"
          className="ed-ppos"
          data-testid="page-position"
          aria-expanded={sheet === "pages"}
          onClick={() => onSheet(sheet === "pages" ? null : "pages")}
        >
          {t("editor.pageOf", { n: index + 1, total: count, name: pageName(t, page) })}
        </button>
        <button type="button" aria-disabled={index === count - 1} onClick={() => onStep(1)}>
          {t("editor.nextPage")}
        </button>
      </div>
      <div className="ed-pager ed-pact" role="group" aria-label={t("editor.phone.actions")}>
        {(["edit", "layers", "add"] as const).map((s) => (
          <button
            key={s}
            type="button"
            aria-expanded={sheet === s}
            disabled={s === "add" && readOnly}
            onClick={() => onSheet(sheet === s ? null : s)}
          >
            {t(s === "edit" ? "editor.phone.edit" : `editor.phone.${s}`)}
          </button>
        ))}
      </div>
    </div>
  );
}

const FOCUSABLE = "button:not([disabled]), a[href], input:not([disabled]), select, textarea";

/**
 * History and Templates on a phone (T-181): a full-height modal dialog over the editor. It takes the focus when it opens, keeps Tab inside, and the
 * editor's Escape handling and its own Close button close it (the editor gives the focus back to the button that opened it). While a version or a template is
 * previewed the dialog is `hidden` (kept mounted, so its list and scroll stay) and the page shows through.
 */
export function FullDialog({
  title,
  onClose,
  hidden,
  children,
}: {
  title: string;
  onClose: () => void;
  hidden: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    box.current?.focus();
  }, []);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Tab" || e.defaultPrevented) return;
    const items = [...(box.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [])];
    const first = items[0];
    const last = items[items.length - 1];
    if (!first || !last) return;
    if (
      e.shiftKey &&
      (document.activeElement === first || document.activeElement === box.current)
    ) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };
  return (
    // the keydown handler is the focus trap; the dialog itself is not a control
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      ref={box}
      className="ed-fulldialog"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      tabIndex={-1}
      hidden={hidden}
      onKeyDown={onKeyDown}
    >
      <div className="ed-fd-head">
        <button type="button" onClick={onClose}>
          {t("editor.phone.close")}
        </button>
      </div>
      <div className="ed-fd-body">{children}</div>
    </div>
  );
}
