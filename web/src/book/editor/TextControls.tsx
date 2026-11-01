import { useTranslation } from "react-i18next";
import type { TemplateLayout } from "../layout/runtime";
import { MAX_TEXT, runeCount, setText, type LayoutItem, type Op } from "./document";
import type { GestureStore } from "./gestures/useGestures";
import { NumField } from "./NumField";
import { Swatches } from "./Swatches";
import { TextField } from "./TextField";

const ALIGNS = ["left", "center", "right"] as const;
export const DEFAULT_SIZE = 24; // what the book draws when a text item has no size (layout/runtime.tsx)

/**
 * The text of a text box in the property panel (T-161): a normal multi-line field with the same cap and paste rules as the one on the page (it is the only
 * way to edit a text that is behind another item), and the style controls: font, size, colour, alignment, bold. Every control writes through `setText`, one
 * history step each (the slider: one step per drag).
 */
export function TextControls({
  item,
  kind,
  spec,
  scope,
  store,
  readOnly,
  cut,
  run,
}: {
  item: LayoutItem;
  kind: string;
  spec: TemplateLayout;
  /** The theme and font set of the book: the colour tokens are custom properties that the template puts on its page. */
  scope: { theme?: string; fonts?: string };
  store: Pick<GestureStore, "preview" | "commit">;
  readOnly: boolean;
  /** The text is still too long at the smallest size the fit rule allows. */
  cut: boolean;
  run: (op: Op) => boolean;
}) {
  const { t } = useTranslation();
  const fonts = Object.keys(spec.fonts);
  const colors = Object.keys(spec.colors);
  const { min, max } = spec.textSize;
  const font = item.font !== undefined && fonts.includes(item.font) ? item.font : "body";
  const color = item.color !== undefined && colors.includes(item.color) ? item.color : "ink";
  const size = Math.min(max, Math.max(min, item.size ?? DEFAULT_SIZE));
  const set = (patch: Parameters<typeof setText>[2]) => run(setText(kind, item.id, patch));
  const align = item.align ?? "left";
  return (
    <fieldset disabled={readOnly} className="ed-text">
      <legend>{t("editor.text")}</legend>
      <TextField
        key={item.id}
        kind={kind}
        id={item.id}
        value={item.text ?? ""}
        store={store}
        label={t("editor.textPanelField")}
        className="ed-panel-text"
        rows={5}
        readOnly={readOnly}
      />
      <p className="ed-hint" aria-live="off">
        {t("editor.runes", { n: runeCount(item.text ?? ""), max: MAX_TEXT })}
      </p>
      {cut && (
        <p role="status" className="ed-warn" data-testid="text-cut-panel">
          {t("editor.textCut")}
        </p>
      )}
      <label className="ed-field">
        <span>{t("editor.font.label")}</span>
        <select value={font} onChange={(e) => set({ font: e.target.value })}>
          {fonts.map((f) => (
            <option key={f} value={f}>
              {t(`editor.font.${f}`, { defaultValue: f })}
            </option>
          ))}
        </select>
      </label>
      <NumField
        key={`size:${size}`}
        label={t("editor.size")}
        value={size}
        step={1}
        min={min}
        max={max}
        disabled={readOnly}
        onCommit={(n) => set({ size: Math.round(n) })}
      />
      <input
        type="range"
        className="ed-slider"
        aria-label={t("editor.sizeSlider")}
        min={min}
        max={max}
        step={1}
        value={size}
        onChange={(e) => store.preview(setText(kind, item.id, { size: Number(e.target.value) }))}
        onPointerUp={() => store.commit()}
        onKeyUp={() => store.commit()}
        onBlur={() => store.commit()}
      />
      <Swatches
        spec={spec}
        scope={scope}
        value={color}
        label={t("editor.color.label")}
        onPick={(c) => set({ color: c })}
      />
      <div role="group" aria-label={t("editor.align.label")} className="ed-row">
        {ALIGNS.map((a) => (
          <button
            key={a}
            type="button"
            aria-pressed={a === align}
            onClick={() => set({ align: a })}
          >
            {t(`editor.align.${a}`)}
          </button>
        ))}
      </div>
      <label className="ed-check">
        <input
          type="checkbox"
          checked={!!item.bold}
          onChange={(e) => set({ bold: e.target.checked })}
        />
        {t("editor.bold")}
      </label>
    </fieldset>
  );
}
