import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { Media } from "../../api/yearbooks";
import type { OnePage } from "../BookPages";
import type { Box, PageBox } from "../layout/geometry";
import type { TemplateLayout } from "../layout/runtime";
import {
  clearOverride,
  MAX_BOX,
  MIN_BOX,
  ops,
  RATIO,
  removeItem,
  reorderItem,
  setBg,
  setItemProps,
  setOverride,
  type LayoutDoc,
  type Op,
  type Refusal,
} from "./document";
import { pageName, targetName } from "./names";
import { BIG_STEP, BIG_TURN, TURN } from "./gestures/keys";
import { Dialog } from "../../components/ui/Dialog";
import type { GestureStore } from "./gestures/useGestures";
import { AssetControls, ImageControls } from "./ItemControls";
import { Swatches } from "./Swatches";
import { TextControls } from "./TextControls";
import { nudge, type Nudge } from "./gestures/math";
import { NumField, tenth } from "./NumField";
import { overrideOf, patchOf, type Target } from "./targets";

const PHONE_STEP = 5; // design px (the page is 816 wide): one small tap of a move arrow
const PHONE_BIG = 40;

/**
 * The property panel (T-159): the numbers, the stacking order and the actions of the selection, or the properties of the page when nothing is selected.
 * Every change goes through the T-158 store as one history step (`apply`). A template element is always written with its whole box (taken from where it
 * is drawn), so an edit of one number does not move the others back to the template's own place.
 */
export function PropertyPanel({
  doc,
  kind,
  page,
  pb,
  selected,
  apply,
  select,
  readOnly,
  announce,
  spec,
  scope,
  store,
  cut,
  onAddText,
  onDuplicate,
  pickImage,
  onMoreBackgrounds,
  phone = false,
}: {
  doc: LayoutDoc;
  kind: string;
  page: OnePage;
  pb: PageBox;
  selected: Target | undefined;
  apply: (op: Op) => Refusal | undefined;
  select: (ids: readonly string[]) => void;
  readOnly: boolean;
  announce: (s: string) => void;
  /** What the text controls need (T-161): the template's tokens, the book's theme and font set, the store for typing, and whether the selected text is cut. */
  spec: TemplateLayout | undefined;
  scope: { theme?: string; fonts?: string };
  store: Pick<GestureStore, "preview" | "commit">;
  cut: boolean;
  /** "Add text" (the toolbar has the same button). */
  onAddText: () => void;
  /** Duplicate the selected item (T-162; Ctrl or Cmd + D does the same). */
  onDuplicate: () => void;
  /** The picker of the book's design images for "replace image" (T-162): the screen renders it and calls back with the chosen one. */
  pickImage: (onPick: (m: Media) => void) => ReactNode;
  /** Opens the palette's Background tab. */
  onMoreBackgrounds: () => void;
  /** The panel of the phone's bottom sheet (T-181): four move arrows with a small and a big step (no handles on a phone), and one confirm before a delete. */
  phone?: boolean;
}) {
  const { t } = useTranslation();
  const [replacing, setReplacing] = useState<string | null>(null); // the id of the image item that is being replaced
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [last, setRefused] = useState<{ id?: string; why: Refusal } | null>(null);
  const refused = last && last.id === selected?.id ? last.why : null; // a refusal belongs to the selection it was made on

  const run = (op: Op) => {
    const r = apply(op);
    setRefused(r ? { id: selected?.id, why: r } : null);
    return !r;
  };

  if (!selected) {
    const bg = doc.pages[kind]?.bg;
    return (
      <aside className="ed-panel" aria-label={t("editor.panel")}>
        <h2>{t("editor.pageProps", { page: pageName(t, page) })}</h2>
        <p className="ed-hint">{t("editor.nothingSelected")}</p>
        <button type="button" disabled={readOnly} onClick={onAddText}>
          {t("editor.addText")}
        </button>
        {spec && (
          <fieldset disabled={readOnly}>
            <legend>{t("editor.palette.bgColour")}</legend>
            <Swatches
              spec={spec}
              scope={scope}
              value={bg?.color}
              label={t("editor.palette.bgColour")}
              onPick={(c) => run(setBg(kind, { color: c }))}
            />
            <button type="button" onClick={onMoreBackgrounds}>
              {t("editor.palette.bgMore")}
            </button>
          </fieldset>
        )}
        {bg && (
          <button type="button" disabled={readOnly} onClick={() => run(setBg(kind, null))}>
            {t("editor.clearBackground")}
          </button>
        )}
        {refused && (
          <p role="alert">
            {t(`editor.refused.${refused}`, { defaultValue: t("editor.refused.other") })}
          </p>
        )}
      </aside>
    );
  }

  const item = selected.item;
  const rect = selected.rect;
  const ov = overrideOf(doc, selected.id);
  /** Writes only the fields the student changed. A template element that has no stored value for a field yet gets the one it is drawn with, so it stays put. */
  const write = (patch: Partial<Box>) => {
    if (item) return run(ops.setItemBox(selected.kind, selected.id, patch));
    const drawn: Partial<Box> = patchOf(rect, selected.r, pb);
    const have = (k: keyof Box) => ov?.[k] !== undefined;
    for (const k of Object.keys(drawn) as (keyof Box)[]) if (have(k)) delete drawn[k];
    return run(setOverride(selected.id, { ...drawn, ...patch }));
  };
  // The limits of the server (document.ts boxOk) in the units the inputs show: % of the page width or height.
  const wSt = (rect.width * 1000) / pb.w; // stored width and height of the box as drawn
  const hSt = (rect.height * 1000) / pb.w;
  const slack = Math.floor((hSt * RATIO) / 2000);
  const field = (
    label: string,
    value: number,
    [min, max]: [number, number],
    make: (n: number) => Partial<Box>,
    step = 0.1,
  ) => (
    <NumField
      key={`${label}:${tenth(value)}`}
      label={label}
      value={value}
      step={step}
      min={min}
      max={max}
      disabled={readOnly}
      onCommit={(n) => write(make(n))}
    />
  );
  /** A button step: the same maths and clamps as the keyboard (gestures/math.ts `nudge`), written as the whole drawn box. */
  const step = (n: Nudge, what: string) => {
    const next = nudge(rect, selected.r, n, pb);
    if (write(patchOf(next.rect, next.deg, pb))) announce(what);
  };
  const list = doc.pages[kind]?.items ?? [];
  const at = item ? list.findIndex((i) => i.id === item.id) : -1;
  const move = (to: number) => {
    if (run(reorderItem(kind, selected.id, to)))
      announce(t("editor.reordered", { n: to + 1, total: list.length }));
  };
  const hasOverride = overrideOf(doc, selected.id) !== undefined;
  const remove = () => {
    if (run(removeItem(kind, selected.id))) {
      select([]);
      announce(t("editor.deleted"));
    }
  };

  return (
    <aside className="ed-panel" aria-label={t("editor.panel")}>
      <h2>{targetName(t, selected)}</h2>
      <fieldset disabled={readOnly}>
        <legend>{t("editor.box")}</legend>
        {field(t("editor.x"), (rect.left / pb.w) * 100, [-wSt / 20, (2000 - wSt) / 20], (n) => ({
          x: n * 10,
        }))}
        {field(
          t("editor.y"),
          (rect.top / pb.h) * 100,
          [
            (-slack - (hSt / 2) * (pb.w / pb.h)) / 10,
            (1000 + slack - (hSt / 2) * (pb.w / pb.h)) / 10,
          ],
          // the stored centre is the new top plus half of the stored height
          (n) => ({ cy: ((n / 100) * pb.h + rect.height / 2) * (1000 / pb.h) }),
        )}
        {field(t("editor.w"), (rect.width / pb.w) * 100, [MIN_BOX / 10, MAX_BOX / 10], (n) => ({
          w: n * 10,
        }))}
        {field(
          t("editor.h"),
          (rect.height / pb.h) * 100,
          [(MIN_BOX * pb.w) / pb.h / 10, (MAX_BOX * pb.w) / pb.h / 10],
          (n) => {
            const px = (n / 100) * pb.h; // the new height; the shown Top stays, so the centre moves
            return { h: (px * 1000) / pb.w, cy: ((rect.top + px / 2) * 1000) / pb.h };
          },
        )}
        {field(t("editor.rotation"), selected.r, [-180, 180], (n) => ({ r: Math.round(n) }), 1)}
      </fieldset>
      {phone && (
        <div role="group" aria-label={t("editor.phone.move")} className="ed-row ed-move">
          {(
            [
              ["Left", -1, 0],
              ["Right", 1, 0],
              ["Up", 0, -1],
              ["Down", 0, 1],
            ] as const
          ).flatMap(([name, x, y]) =>
            ([["", PHONE_STEP] as const, ["Big", PHONE_BIG] as const] as const).map(([big, n]) => (
              <button
                key={name + big}
                type="button"
                disabled={readOnly}
                onClick={() =>
                  step({ kind: "move", dx: x * n, dy: y * n }, t(`editor.phone.moved${name}${big}`))
                }
              >
                {t(`editor.phone.move${name}${big}`)}
              </button>
            )),
          )}
        </div>
      )}
      <div role="group" aria-label={t("editor.nudge")} className="ed-row">
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "rotate", ddeg: -TURN }, t("editor.nudged.rotateLeft"))}
        >
          {t("editor.rotateLeft")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "rotate", ddeg: TURN }, t("editor.nudged.rotateRight"))}
        >
          {t("editor.rotateRight")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "rotate", ddeg: -BIG_TURN }, t("editor.nudged.rotateLeft"))}
        >
          {t("editor.rotateLeftBig")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "rotate", ddeg: BIG_TURN }, t("editor.nudged.rotateRight"))}
        >
          {t("editor.rotateRightBig")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "resize", dw: BIG_STEP, dh: 0 }, t("editor.nudged.wider"))}
        >
          {t("editor.wider")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() =>
            step({ kind: "resize", dw: -BIG_STEP, dh: 0 }, t("editor.nudged.narrower"))
          }
        >
          {t("editor.narrower")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "resize", dw: 0, dh: BIG_STEP }, t("editor.nudged.taller"))}
        >
          {t("editor.taller")}
        </button>
        <button
          type="button"
          disabled={readOnly}
          onClick={() => step({ kind: "resize", dw: 0, dh: -BIG_STEP }, t("editor.nudged.shorter"))}
        >
          {t("editor.shorter")}
        </button>
      </div>
      {refused && (
        <p role="alert">
          {t(`editor.refused.${refused}`, { defaultValue: t("editor.refused.other") })}
        </p>
      )}
      {item ? (
        <>
          {item.t === "text" && spec && (
            <TextControls
              item={item}
              kind={kind}
              spec={spec}
              scope={scope}
              store={store}
              readOnly={readOnly}
              cut={cut}
              run={run}
            />
          )}
          {item.t === "asset" && spec && (
            <AssetControls
              item={item}
              kind={kind}
              spec={spec}
              scope={scope}
              readOnly={readOnly}
              run={run}
            />
          )}
          {item.t === "image" && (
            <ImageControls
              item={item}
              kind={kind}
              readOnly={readOnly}
              run={run}
              store={store}
              onReplace={() => setReplacing(replacing === item.id ? null : item.id)}
            />
          )}
          {item.t === "image" &&
            replacing === item.id &&
            pickImage((m) => {
              if (run(setItemProps(kind, item.id, { media: m.id }))) {
                setReplacing(null);
                announce(t("editor.image.replaced"));
              }
            })}
          <div role="group" aria-label={t("editor.order")} className="ed-row">
            <button
              type="button"
              disabled={readOnly || at >= list.length - 1}
              onClick={() => move(list.length - 1)}
            >
              {t("editor.toFront")}
            </button>
            <button
              type="button"
              disabled={readOnly || at >= list.length - 1}
              onClick={() => move(at + 1)}
            >
              {t("editor.forward")}
            </button>
            <button type="button" disabled={readOnly || at <= 0} onClick={() => move(at - 1)}>
              {t("editor.backward")}
            </button>
            <button type="button" disabled={readOnly || at <= 0} onClick={() => move(0)}>
              {t("editor.toBack")}
            </button>
          </div>
          <button type="button" disabled={readOnly} onClick={onDuplicate}>
            {t("editor.duplicate")}
          </button>
          <button
            type="button"
            className="ed-danger"
            disabled={readOnly}
            onClick={() => (phone ? setConfirmDelete(true) : remove())}
          >
            {t("editor.delete")}
          </button>
          {confirmDelete && (
            <Dialog
              title={t("editor.phone.deleteTitle")}
              confirmLabel={t("editor.delete")}
              cancelLabel={t("editor.phone.cancel")}
              danger
              onConfirm={() => {
                setConfirmDelete(false);
                remove();
              }}
              onCancel={() => setConfirmDelete(false)}
            >
              <p>{t("editor.phone.deleteBody", { name: targetName(t, selected) })}</p>
            </Dialog>
          )}
        </>
      ) : (
        <div className="ed-row">
          <button
            type="button"
            disabled={readOnly}
            onClick={() => run(setOverride(selected.id, { hidden: !selected.hidden }))}
          >
            {selected.hidden ? t("editor.show") : t("editor.hide")}
          </button>
          <button
            type="button"
            disabled={readOnly || !hasOverride}
            onClick={() => run(clearOverride(selected.id))}
          >
            {t("editor.resetElement")}
          </button>
        </div>
      )}
    </aside>
  );
}
