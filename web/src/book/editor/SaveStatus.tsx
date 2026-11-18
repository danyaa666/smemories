import { useTranslation } from "react-i18next";
import type { SaveState } from "./store";

/**
 * One line for the save state of the layout editor (T-158); the editor screen (T-159) places it. "Saving..." and "Saved" are routine, they come at every autosave
 * (every pause in the editing), so they are shown but not announced (T-182: a screen reader would repeat them all session); a state the student has to know about
 * (offline, failing, conflict, invalid, gone) is a polite status message.
 */
export function SaveStatus({ state, dirty }: { state: SaveState; dirty: boolean }) {
  const { t } = useTranslation();
  // idle with edits waiting for the autosave still reads "saving"; idle and clean says nothing
  const key = state === "idle" ? (dirty ? "saving" : null) : state;
  const routine = key === null || key === "saving" || key === "saved";
  return (
    <span data-save-state={state}>
      <span aria-live="off">{routine && key ? t(`editor.save.${key}`) : ""}</span>
      <span role="status" aria-live="polite">
        {routine || !key ? "" : t(`editor.save.${key}`)}
      </span>
    </span>
  );
}
