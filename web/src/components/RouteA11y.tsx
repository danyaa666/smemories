import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";

export const MAIN_ID = "main-content";

/** First Tab stop of a layout: moves focus to <main id="main-content"> (F-08). */
export function SkipLink() {
  const { t } = useTranslation();
  return (
    <a
      href={`#${MAIN_ID}`}
      className="skip-link"
      onClick={(e) => {
        // A real hash change would fight the router, so move focus by hand.
        e.preventDefault();
        document.getElementById(MAIN_ID)?.focus();
      }}
    >
      {t("nav.skip")}
    </a>
  );
}

/**
 * After a client-side navigation, moves focus to the new page's <h1> so a screen reader says the page name (F-09).
 * Not on the first load, and only when the path changes (dialogs, tabs and query changes keep the focus).
 * Pages load lazily or after a fetch, so we wait for the <h1> for up to 3 s.
 */
export function RouteFocus() {
  const { pathname } = useLocation();
  const first = useRef(pathname);
  useEffect(() => {
    if (first.current === pathname) return;
    first.current = pathname;
    const focusH1 = () => {
      const h1 = document.querySelector<HTMLElement>(`#${MAIN_ID} h1, main h1`);
      if (!h1) return false;
      // A page that already took the focus itself (T-170 on the manage page) keeps it.
      const now = document.activeElement;
      if (now && now !== h1 && h1.closest("main")?.contains(now) && now !== h1.closest("main"))
        return true;
      h1.setAttribute("tabindex", "-1");
      h1.focus();
      return true;
    };
    if (focusH1()) return;
    const seen = new MutationObserver(() => focusH1() && seen.disconnect());
    seen.observe(document.body, { childList: true, subtree: true });
    const stop = setTimeout(() => seen.disconnect(), 3000);
    return () => {
      seen.disconnect();
      clearTimeout(stop);
    };
  }, [pathname]);
  return null;
}
