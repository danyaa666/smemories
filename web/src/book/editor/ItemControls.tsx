import { useTranslation } from "react-i18next";
import { getAsset } from "../assets";
import type { TemplateLayout } from "../layout/runtime";
import { setItemProps, type LayoutItem, type Op } from "./document";
import type { GestureStore } from "./gestures/useGestures";
import { Swatches } from "./Swatches";

type Common = {
  item: LayoutItem;
  kind: string;
  readOnly: boolean;
  run: (op: Op) => boolean;
};

/** The tint of a sticker, shape or frame (T-162): the colour tokens and "original colours". A picture that paints its own colours has no tint control. */
export function AssetControls({
  item,
  kind,
  spec,
  scope,
  readOnly,
  run,
}: Common & { spec: TemplateLayout; scope: { theme?: string; fonts?: string } }) {
  const { t } = useTranslation();
  const asset = item.asset ? getAsset(item.asset) : undefined;
  if (!asset?.tintable) return null;
  const tint = item.tint !== undefined && item.tint in spec.colors ? item.tint : undefined;
  return (
    <fieldset disabled={readOnly}>
      <legend>{t("editor.tint.label")}</legend>
      <Swatches
        spec={spec}
        scope={scope}
        value={tint}
        label={t("editor.tint.label")}
        onPick={(c) => run(setItemProps(kind, item.id, { tint: c }))}
      />
      <button
        type="button"
        disabled={readOnly || item.tint === undefined}
        onClick={() => run(setItemProps(kind, item.id, { tint: undefined }))}
      >
        {t("editor.tint.original")}
      </button>
    </fieldset>
  );
}

/** Fit, corner radius and "replace image" of an image item (T-162). The file name is never shown: the client name is not stored. */
export function ImageControls({
  item,
  kind,
  readOnly,
  run,
  store,
  onReplace,
}: Common & { store: Pick<GestureStore, "preview" | "commit">; onReplace: () => void }) {
  const { t } = useTranslation();
  const radius = item.radius ?? 0;
  return (
    <fieldset disabled={readOnly}>
      <legend>{t("editor.image.label")}</legend>
      <label className="ed-field">
        <span>{t("editor.image.fit")}</span>
        <select
          value={item.fit ?? "cover"}
          onChange={(e) =>
            run(setItemProps(kind, item.id, { fit: e.target.value as "cover" | "contain" }))
          }
        >
          <option value="contain">{t("editor.image.contain")}</option>
          <option value="cover">{t("editor.image.cover")}</option>
        </select>
      </label>
      <label className="ed-field">
        <span>{t("editor.image.radius")}</span>
        <output>{radius}</output>
      </label>
      <input
        type="range"
        className="ed-slider"
        aria-label={t("editor.image.radius")}
        min={0}
        max={500}
        step={1}
        value={radius}
        onChange={(e) =>
          store.preview(setItemProps(kind, item.id, { radius: Number(e.target.value) }))
        }
        onPointerUp={() => store.commit()}
        onKeyUp={() => store.commit()}
        onBlur={() => store.commit()}
      />
      <button type="button" disabled={readOnly} onClick={onReplace}>
        {t("editor.image.replace")}
      </button>
    </fieldset>
  );
}
