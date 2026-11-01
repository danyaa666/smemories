import type { TFunction } from "i18next";
import type { OnePage } from "../BookPages";
import type { Target } from "./targets";

/** The words for a page of the book ("Cover", "Notes page 2"); the number counts the notes pages only. */
export const pageName = (t: TFunction, p: OnePage) =>
  p.section === "notes"
    ? t("editor.page.notes", { n: (p.index ?? 0) + 1 })
    : t(`editor.page.${p.section}`);

/** The accessible name of a selectable thing: an element of the template by its layout id, or an item by its kind (a text by its start). */
export function targetName(t: TFunction, x: Target): string {
  const base = x.item
    ? t(`editor.item.${x.item.t}`, {
        text: Array.from(x.item.text ?? "")
          .slice(0, 30)
          .join(""),
      })
    : t(`editor.element.${x.id}`, { defaultValue: x.id });
  return x.hidden ? t("editor.hiddenName", { name: base }) : base;
}
