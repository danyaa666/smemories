import type { CSSProperties, FocusEvent } from "react";
import { useTranslation } from "react-i18next";
import type { FitInfo } from "../fit";
import type { LayoutItem } from "./document";
import type { GestureStore } from "./gestures/useGestures";
import { TextField } from "./TextField";

const MIN_TOUCH_PX = 16; // a field below this makes iOS Safari zoom the page when it gets the focus

/**
 * The text field laid over a printed text box (T-161). It is drawn inside the box (`layout/runtime.tsx` TextItem), so the rotation, the clip, the page scale and the
 * font set come from the same ancestors as the printed text, and it takes the printed text's own style and the size the fit rule chose: the edit view is the
 * printed view. On a touch screen its real font size is never below 16 css px; a smaller text is drawn with the field scaled down by a transform instead.
 */
export function PageTextField({
  item,
  kind,
  style,
  fit,
  store,
  selectAll,
  readOnly,
  onDone,
}: {
  item: LayoutItem;
  kind: string;
  style: CSSProperties;
  fit: FitInfo | undefined;
  store: Pick<GestureStore, "preview" | "commit">;
  selectAll: boolean;
  readOnly: boolean;
  /** Edit mode ends: by Escape (`blurred` undefined) or because the focus left the field (the element it went to, or null). */
  onDone: (id: string, to: { escape: true } | { blurred: EventTarget | null }) => void;
}) {
  const { t } = useTranslation();
  const px = fit?.px ?? Number(style.fontSize);
  const coarse = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches;
  const css: CSSProperties = { ...style, fontSize: px, width: "100%", height: "100%" };
  if (coarse && px < MIN_TOUCH_PX) {
    const f = MIN_TOUCH_PX / px; // the field is f times the box, drawn 1/f as large
    Object.assign(css, {
      fontSize: MIN_TOUCH_PX,
      width: `${f * 100}%`,
      height: `${f * 100}%`,
      transform: `scale(${1 / f})`,
      transformOrigin: "0 0",
    });
  }
  return (
    <TextField
      kind={kind}
      id={item.id}
      value={item.text ?? ""}
      store={store}
      label={t("editor.textField")}
      className="ed-textarea"
      style={css}
      readOnly={readOnly}
      focus
      selectAll={selectAll}
      onEscape={() => onDone(item.id, { escape: true })}
      onBlur={(e: FocusEvent<HTMLTextAreaElement>) => onDone(item.id, { blurred: e.relatedTarget })}
    />
  );
}
