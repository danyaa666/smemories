import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { SchoolClass } from "../api/classes";
import { buttonClass } from "../components/ui/Button";
import { CreatePageButton } from "./CreatePageButton";

/** The caller's own page in the class: all a student ever sees about pages (class/get carries nobody else's). */
export function MyPage({ cls }: { cls: SchoolClass }) {
  const { t } = useTranslation();
  const page = cls.page;
  return (
    <section aria-labelledby="my-page">
      <h2 id="my-page">{t("classes.page.title")}</h2>
      {page?.linked && page.yearbook_id ? (
        <>
          <ul>
            <li>
              {t(page.profile_complete ? "classes.page.complete" : "classes.page.incomplete")}
            </li>
            <li>{t("classes.page.notes", { n: page.approved_notes })}</li>
          </ul>
          <p>
            <Link to={`/yearbooks/${page.yearbook_id}`} className={buttonClass("primary")}>
              {t("classes.page.open")}
            </Link>
          </p>
        </>
      ) : (
        <>
          <p>{t("classes.page.none")}</p>
          <CreatePageButton cls={cls} />
        </>
      )}
    </section>
  );
}
