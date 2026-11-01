import { useTranslation } from "react-i18next";
import type { TemplateLayout } from "../layout/runtime";

/**
 * A row of the template's colour tokens as round swatches (a radio group), shared by the text colour (T-161), the tint of a sticker and the page background
 * (T-162). `value` is the chosen token or undefined for none; the swatches take the book's theme through the custom properties the template puts on its page.
 */
export function Swatches({
  spec,
  scope,
  value,
  label,
  onPick,
}: {
  spec: TemplateLayout;
  scope: { theme?: string; fonts?: string };
  value: string | undefined;
  label: string;
  onPick: (token: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={["mb-layer", spec.layerClass, "ed-swatches"].filter(Boolean).join(" ")}
      data-theme={scope.theme || undefined}
      data-fonts={scope.fonts || undefined}
      role="radiogroup"
      aria-label={label}
    >
      {Object.keys(spec.colors).map((c) => (
        <button
          key={c}
          type="button"
          role="radio"
          aria-checked={c === value}
          aria-label={t(`editor.color.${c}`, { defaultValue: c })}
          title={t(`editor.color.${c}`, { defaultValue: c })}
          className="ed-swatch"
          style={{ background: spec.colors[c] }}
          onClick={() => onPick(c)}
        />
      ))}
    </div>
  );
}
