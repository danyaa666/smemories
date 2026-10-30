import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

/**
 * Asks before the user leaves a screen with unsaved changes. The app uses a plain BrowserRouter (no data router, so no `useBlocker`), so this
 * covers what it can: closing or reloading the tab (the browser's own prompt) and a click on any in-app link (`pending` holds the target while a
 * dialog asks). Programmatic navigation and the browser's Back button are not intercepted.
 */
export function useLeaveGuard(active: boolean) {
  const navigate = useNavigate();
  const [pending, setPending] = useState<string | null>(null);
  const off = useRef(false); // set while a confirmed leave is under way

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
    /** The place the user wants to go to, while the dialog asks. */
    pending,
    stay: () => setPending(null),
    leave: () => {
      off.current = true;
      if (pending !== null) void navigate(pending);
      setPending(null);
    },
  };
}
