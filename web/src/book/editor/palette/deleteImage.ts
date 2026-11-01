// Deleting a design image (T-162, spec 21 requirement 7). The server refuses a layout that points at a missing image, and undo could bring such a layout
// back, so an image that is in use goes in this order: (a) one step removes every item and background that uses it, (b) wait until that is saved,
// (c) delete the image, (d) forget the undo and redo steps. An image that is in no page and no step of the history is just deleted and the history stays.
// An image that only the history points at (an item removed earlier, T-164) skips (a) and (b) but still clears the history, or undo would bring it back.
import { mediaUses, purgeMedia, type LayoutDoc, type Op, type Refusal } from "../document";

export type DeleteDeps = {
  doc: () => LayoutDoc;
  apply: (op: Op) => Refusal | undefined;
  /** Resolves true when the server has the newest document (`store.settle`). */
  settle: () => Promise<boolean>;
  clearHistory: () => void;
  /** True when an undo or redo step (or the document) still uses the image. */
  usesInHistory: (id: string) => boolean;
  deleteMedia: (id: string) => Promise<unknown>;
};

/**
 * `saved`: the layout could not be saved, so the image was NOT deleted (the pages already lost their use of it on this screen and the autosave retries).
 * `failed`: the layout is saved without the image but the delete call failed; the image stays in the list and nothing is lost on the server.
 */
export type DeleteResult =
  { ok: true; cleared: boolean } | { ok: false; stage: "save" | "delete"; error?: unknown };

export async function deleteDesignImage(id: string, d: DeleteDeps): Promise<DeleteResult> {
  const onPage = mediaUses(d.doc(), id).length > 0;
  const used = onPage || d.usesInHistory(id);
  if (onPage) {
    d.apply(purgeMedia(id));
    if (!(await d.settle())) return { ok: false, stage: "save" };
  }
  try {
    await d.deleteMedia(id);
  } catch (error) {
    return { ok: false, stage: "delete", error };
  }
  if (used) d.clearHistory();
  return { ok: true, cleared: used };
}
