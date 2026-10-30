import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

/**
 * Asks before the user leaves a screen with unsaved changes. The app uses a plain BrowserRouter (no data router, so no `useBlocker`), so this
 * covers what it can: closing or reloading the tab (the browser's own prompt), a click on any in-app link (`pending` holds the target while a
 * dialog asks) and the browser's or phone's Back (T-148). Programmatic navigation is not intercepted.
 *
 * Back works with a sentinel: while `active`, one extra history entry with the same URL sits on top of the stack, so Back only pops to the
 * screen's own entry (same route, so the screen and its edits stay mounted) and the dialog can ask. "Stay" costs nothing (the sentinel is put back
 * at once); "Leave" goes two entries back. When `active` turns off (saved, or edits undone) the sentinel is removed again.
 * ponytail: a history jump of several entries at once (long-press Back) skips the dialog.
 */
export function useLeaveGuard(active: boolean) {
  const navigate = useNavigate();
  const [pending, setPending] = useState<string | null>(null);
  const [back, setBack] = useState(false); // Back was pressed: the dialog asks
  const off = useRef(false); // set while a confirmed leave is under way
  const armed = useRef(false); // the sentinel entry is on top of the history stack

  useEffect(() => {
    if (!active) {
      if (armed.current) {
        armed.current = false;
        window.history.back(); // pops the sentinel; the screen's own entry has the same URL
      }
      return;
    }
    if (!armed.current) {
      window.history.pushState(window.history.state, "", window.location.href);
      armed.current = true;
    }
    const here = window.location.href;
    const pop = () => {
      if (off.current || window.location.href !== here) return;
      window.history.pushState(window.history.state, "", here); // the sentinel again, so a second Back asks again
      setBack(true);
    };
    window.addEventListener("popstate", pop);
    return () => window.removeEventListener("popstate", pop);
  }, [active]);

  useEffect(() => {
    if (!active) return;
    const unload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = ""; // some browsers need it to show the prompt
    };
    const click = (e: MouseEvent) => {
      if (off.current || e.defaultPrevented || e.button !== 0) return;
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return; // a new tab or window leaves this one alone
      const a = e.target instanceof Element ? e.target.closest("a[href]") : null;
      if (!(a instanceof HTMLAnchorElement) || (a.target && a.target !== "_self")) return;
      const to = new URL(a.href, window.location.href);
      if (to.origin !== window.location.origin) return;
      if (to.pathname + to.search === window.location.pathname + window.location.search) return;
      e.preventDefault();
      e.stopPropagation(); // before the router's own handler sees it
      setPending(to.pathname + to.search + to.hash);
    };
    window.addEventListener("beforeunload", unload);
    document.addEventListener("click", click, true);
    return () => {
      window.removeEventListener("beforeunload", unload);
      document.removeEventListener("click", click, true);
    };
  }, [active]);

  return {
    /** True while the dialog asks (a link was clicked or Back was pressed). */
    pending: pending !== null || back,
    stay: () => {
      setPending(null);
      setBack(false);
    },
    leave: () => {
      off.current = true;
      if (pending !== null)
        void navigate(pending, { replace: armed.current }); // replaces the sentinel: Back from there returns to this screen
      else window.history.go(-2); // over the sentinel and this screen's own entry
      setPending(null);
      setBack(false);
    },
  };
}
