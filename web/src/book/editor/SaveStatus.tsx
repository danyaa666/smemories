import { useTranslation } from "react-i18next";
import type { SaveState } from "./store";

/** One line for the save state of the layout editor (T-158); the editor screen (T-159) places it. */
export function SaveStatus({ state, dirty }: { state: SaveState; dirty: boolean }) {
  const { t } = useTranslation();
  // idle with edits waiting for the autosave still reads "saving"; idle and clean says nothing
  const key = state === "idle" ? (dirty ? "saving" : null) : state;
  return (
    <span role="status" aria-live="polite" data-save-state={state}>
      {key ? t(`editor.save.${key}`) : ""}
    </span>
  );
}
