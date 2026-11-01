import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useLeaveGuard } from "../../components/useLeaveGuard";
import { EditorStore } from "./store";

/**
 * The state of the layout editor of one book (epic E09 stage 2, T-158; the screen is T-159): document, undo/redo, autosave, conflict.
 * Edits are `editor.apply(ops.setItemBox(...))` (one history step) or `begin()`, `preview(op)`..., `commit()` (a gesture). `leaveGuard` is the
 * `useLeaveGuard` of the customise screen (T-148), active while there is anything unsaved; the screen renders its dialog.
 */
export function useLayoutEditor(yearbookId: string) {
  const store = useMemo(() => new EditorStore(yearbookId), [yearbookId]);
  const state = useSyncExternalStore(store.subscribe, store.getSnapshot);

  useEffect(() => {
    store.start();
    // A hidden page (tab switch, app switch on a phone) may never come back: save now. Back online: retry now.
    const hide = () => {
      if (document.visibilityState === "hidden") store.flush();
    };
    document.addEventListener("visibilitychange", hide);
    window.addEventListener("online", store.flush);
    return () => {
      document.removeEventListener("visibilitychange", hide);
      window.removeEventListener("online", store.flush);
      store.stop();
    };
  }, [store]);

  const leaveGuard = useLeaveGuard(state.dirty);

  // stable for the life of the store, so screens can use them as effect dependencies
  const actions = useMemo(
    () => ({
      apply: store.apply.bind(store),
      begin: store.begin.bind(store),
      preview: store.preview.bind(store),
      commit: store.commit.bind(store),
      cancel: store.cancel.bind(store),
      undo: store.undo.bind(store),
      redo: store.redo.bind(store),
      clearHistory: store.clearHistory.bind(store),
      flush: store.flush,
      settle: store.settle,
      select: store.select,
      clearSelection: () => store.select([]),
      useTheirs: store.useTheirs.bind(store),
      keepMine: store.keepMine.bind(store),
      reset: store.reset.bind(store),
      reload: store.load.bind(store),
    }),
    [store],
  );

  return { ...state, ...actions, leaveGuard };
}
