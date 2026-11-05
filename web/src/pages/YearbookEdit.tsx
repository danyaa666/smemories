import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router-dom";
import { getBookData } from "../api/book";
import { listPages } from "../book/BookPages";
import { getTemplate } from "../book/templates";
import { deleteYearbook, getYearbook, updateYearbook, type Yearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { BookForm } from "../components/BookForm";
import { Photos } from "../components/Photos";
import { ProfileForm } from "../components/ProfileForm";
import { YEARBOOKS_KEY } from "./Yearbooks";
import { Button } from "../components/ui/Button";

export function YearbookEdit() {
  const { t } = useTranslation();
  const id = useParams().id ?? "";
  const qc = useQueryClient();
  const navigate = useNavigate();
  const q = useQuery({ queryKey: ["yearbook", id], queryFn: () => getYearbook(id) });
  // the page kinds the editor shows (a hidden section, or notes while there are none, has no link); a failed read leaves the first link alone
  const layout = !!q.data && !!getTemplate(q.data.template_id).layout;
  const book = useQuery({
    queryKey: ["book-data", id],
    queryFn: () => getBookData(id),
    enabled: layout,
    retry: false,
    refetchOnWindowFocus: false,
  });
  // T-170: arriving here (after Done in the editor, or from a link) puts focus on the page heading, not on whatever button the old page left behind
  const heading = useRef<HTMLHeadingElement>(null);
  const loaded = q.isSuccess;
  useEffect(() => {
    if (loaded) heading.current?.focus();
  }, [loaded]);
  const saved = (y: Yearbook) => {
    qc.setQueryData(["yearbook", id], y);
    void qc.invalidateQueries({ queryKey: YEARBOOKS_KEY });
  };
  const save = useMutation({
    // no cover_media_id here: PATCH leaves it as it is when the key is absent
    mutationFn: (body: Parameters<typeof updateYearbook>[1]) => updateYearbook(id, body),
    onSuccess: saved,
  });
  const del = useMutation({
    mutationFn: () => deleteYearbook(id),
    onSuccess: () => {
      qc.removeQueries({ queryKey: ["yearbook", id] });
      void qc.invalidateQueries({ queryKey: YEARBOOKS_KEY });
      void navigate("/yearbooks");
    },
  });

  if (q.isPending) return <p role="status">{t("auth.loading")}</p>;
  if (q.isError) {
    // a missing or malformed book id never gets here: YearbookLayout shows BookNotFound instead
    return (
      <>
        <p role="alert" className="error">
          {errorText(t, q.error)}
        </p>
        <Button type="button" onClick={() => void q.refetch()}>
          {t("auth.retry")}
        </Button>
        <p>
          <Link to="/yearbooks">{t("books.back")}</Link>
        </p>
      </>
    );
  }
  const y = q.data;
  return (
    <>
      <p>
        <Link to="/yearbooks">{t("books.back")}</Link>
      </p>
      <h1 ref={heading} tabIndex={-1}>
        {y.title}
      </h1>
      <p>
        <Link to={`/yearbooks/${y.id}/customise`}>{t("customise.link")}</Link>
      </p>
      {layout && (
        <section aria-labelledby="pages-h">
          <h2 id="pages-h">{t("designPages.heading")}</h2>
          <p>{t("designPages.intro")}</p>
          <ul>
            <li>
              <Link to={`/yearbooks/${y.id}/editor`}>{t("designPages.all")}</Link>
            </li>
            {[...new Set(book.data ? listPages(book.data).map((p) => p.section) : [])].map((s) => (
              <li key={s}>
                <Link to={`/yearbooks/${y.id}/editor?page=${s}`}>{t(`designPages.${s}`)}</Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section aria-labelledby="book-h">
        <h2 id="book-h">{t("book.heading")}</h2>
        <BookForm
          initial={y}
          submitLabel={t("book.save")}
          pending={save.isPending}
          error={save.error}
          onSubmit={(b) => save.mutate(b)}
        />
        {save.isSuccess && <p role="status">{t("book.saved")}</p>}
      </section>
      <section aria-labelledby="profile-h">
        <h2 id="profile-h">{t("profile.heading")}</h2>
        <ProfileForm yearbook={y} onSaved={saved} />
      </section>
      <Photos yearbook={y} onChanged={saved} />
      <section aria-labelledby="danger-h">
        <h2 id="danger-h">{t("book.dangerHeading")}</h2>
        <p>{t("book.deleteWarning")}</p>
        {del.isError && (
          <p role="alert" className="error">
            {errorText(t, del.error)}
          </p>
        )}
        <Button
          variant="danger"
          busy={del.isPending}
          onClick={() =>
            window.confirm(t("book.confirmDelete", { title: y.title })) && del.mutate()
          }
        >
          {t("book.delete")}
        </Button>
      </section>
    </>
  );
}
