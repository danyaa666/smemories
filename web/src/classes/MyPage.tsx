import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { setBook, type SchoolClass } from "../api/classes";
import { Button, buttonClass } from "../components/ui/Button";
import { classErrorText } from "./classErrors";
import { CreatePageButton } from "./CreatePageButton";
import { PagePicker } from "./PagePicker";

/** The caller's own page in the class: all a student ever sees about pages (class/get carries nobody else's). */
export function MyPage({ cls }: { cls: SchoolClass }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const page = cls.page;
  const linked = page?.linked && page.yearbook_id;
  const [picking, setPicking] = useState(false);
  const unlink = useMutation({
    mutationFn: () => setBook(cls.id, null),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["class", cls.id] }),
  });
  return (
    <section aria-labelledby="my-page">
      <h2 id="my-page">{t("classes.page.title")}</h2>
      {linked ? (
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
        <p>{t("classes.page.none")}</p>
      )}
      {unlink.error && (
        <p role="alert" className="error">
          {classErrorText(t, unlink.error, "page")}
        </p>
      )}
      {!linked && <CreatePageButton cls={cls} />}
      <div className="row-actions">
        {!picking && (
          <Button onClick={() => setPicking(true)}>
            {t(linked ? "classes.pick.change" : "classes.pick.choose")}
          </Button>
        )}
        {linked && (
          <Button busy={unlink.isPending} onClick={() => unlink.mutate()}>
            {t("classes.pick.unlink")}
          </Button>
        )}
      </div>
      {picking && (
        <PagePicker
          classId={cls.id}
          currentId={page?.yearbook_id}
          onDone={() => setPicking(false)}
          onCancel={() => setPicking(false)}
        />
      )}
    </section>
  );
}
