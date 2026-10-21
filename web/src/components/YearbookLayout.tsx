import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, NavLink, Outlet, useMatch } from "react-router-dom";
import { getYearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { useMe } from "../auth/useMe";
import { useNoteCounts } from "./useNoteCounts";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { LegalLinks } from "./LegalConsent";
import { Badge } from "./ui/Badge";
import { Button } from "./ui/Button";
import { useMediaQuery } from "./useMediaQuery";
import { useSignOut } from "./useSignOut";

export type NavItem = {
  key: string;
  /** The link target, or null when it needs a yearbook and none is open. */
  to: (bookId: string | undefined) => string | null;
  /** i18n keys: sidebar label and (shorter) tab bar label. */
  label: string;
  tab: string;
  /** Pending count, shown as a badge on the sidebar item. */
  badge?: number;
};

/**
 * One entry per screen that exists. An item whose route does not exist yet is simply not listed.
 */
export const NAV_ITEMS: NavItem[] = [
  { key: "books", to: () => "/yearbooks", label: "nav.yearbooks", tab: "nav.tabBooks" },
  {
    key: "profile",
    to: (id) => (id ? `/yearbooks/${id}` : null),
    label: "nav.profile",
    tab: "nav.tabProfile",
  },
  {
    key: "notes",
    to: (id) => (id ? `/yearbooks/${id}/notes` : null),
    label: "nav.notes",
    tab: "nav.tabNotes",
  },
  {
    key: "links",
    to: (id) => (id ? `/yearbooks/${id}/note-links` : null),
    label: "nav.links",
    tab: "nav.tabLinks",
  },
  {
    key: "book",
    to: (id) => (id ? `/yearbooks/${id}/book` : null),
    label: "nav.print",
    tab: "nav.tabPrint",
  },
];

/** The signed-in shell: a 256 px sidebar from 960 px up, a bottom tab bar below. Only one of them is in the page. */
export function YearbookLayout({ items = NAV_ITEMS }: { items?: NavItem[] }) {
  const { t } = useTranslation();
  const wide = useMediaQuery("(min-width: 960px)");
  const { data: user } = useMe();
  const out = useSignOut();
  const matched = useMatch("/yearbooks/:id/*")?.params.id;
  const bookId = matched && matched !== "new" ? matched : undefined;
  const book = useQuery({
    queryKey: ["yearbook", bookId],
    queryFn: () => getYearbook(bookId ?? ""),
    enabled: bookId !== undefined,
  });
  const pending = useNoteCounts(bookId).data?.pending;
  const links = items.flatMap((i) => {
    const to = i.to(bookId);
    return to === null
      ? []
      : [{ ...i, to, badge: i.key === "notes" && pending !== undefined ? pending : i.badge }];
  });

  const handle = user?.username ?? "";
  const chip = (
    <div className="user-chip">
      <Link to="/account" className="user-chip-name" title={user?.email}>
        <span className="avatar" aria-hidden="true">
          {handle.slice(0, 1).toUpperCase()}
        </span>
        <span>{handle}</span>
      </Link>
      <Button variant="ghost" busy={out.isPending} onClick={() => out.mutate()}>
        {t("nav.signOut")}
      </Button>
      {out.isError && <span role="alert">{errorText(t, out.error)}</span>}
    </div>
  );
  const brand = (
    <Link to="/yearbooks" className="brand">
      {t("app.name")}
    </Link>
  );

  if (!wide) {
    return (
      <div className="shell shell-narrow">
        <header className="topbar">
          {brand}
          {chip}
          <LanguageSwitcher />
        </header>
        <main>
          <Outlet />
        </main>
        <footer className="shell-footer">
          <LegalLinks />
        </footer>
        <nav className="tabbar" aria-label={t("nav.main")}>
          {links.map((i) => (
            <NavLink key={i.key} to={i.to} end>
              {t(i.tab)}
            </NavLink>
          ))}
        </nav>
      </div>
    );
  }
  return (
    <div className="shell shell-wide">
      <aside className="sidebar">
        {brand}
        {book.data && (
          <Link to={`/yearbooks/${book.data.id}`} className="book-card">
            <strong>{book.data.title}</strong>
            <small>
              {[book.data.school_name, book.data.class_name].filter(Boolean).join(" · ")}
            </small>
          </Link>
        )}
        <nav aria-label={t("nav.main")}>
          <ul>
            {links.map((i) => (
              <li key={i.key}>
                <NavLink to={i.to} end>
                  {t(i.label)}
                  {i.badge ? (
                    <>
                      {" "}
                      <Badge variant="pending">{i.badge}</Badge>
                    </>
                  ) : null}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
        <div className="sidebar-foot">
          {chip}
          <LanguageSwitcher />
          <LegalLinks />
        </div>
      </aside>
      <main>
        <Outlet />
      </main>
    </div>
  );
}
