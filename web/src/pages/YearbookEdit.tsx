import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { deleteYearbook, getYearbook, updateYearbook, type Yearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { BookForm } from "../components/BookForm";
import { Photos } from "../components/Photos";
import { ProfileForm } from "../components/ProfileForm";
import { YEARBOOKS_KEY } from "./Yearbooks";

export function YearbookEdit() {
  const { t } = useTranslation();
  const id = useParams().id ?? "";
  const qc = useQueryClient();
  const navigate = useNavigate();
  const q = useQuery({ queryKey: ["yearbook", id], queryFn: () => getYearbook(id) });
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
    const gone = q.error instanceof ApiError && q.error.status === 404;
    return (
      <>
        <p role="alert" className="error">
          {gone ? t("books.notFound") : errorText(t, q.error)}
        </p>
        {!gone && (
          <button type="button" onClick={() => void q.refetch()}>
            {t("auth.retry")}
          </button>
        )}
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
      <h1>{y.title}</h1>
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
        <button
          type="button"
          disabled={del.isPending}
          onClick={() =>
            window.confirm(t("book.confirmDelete", { title: y.title })) && del.mutate()
          }
        >
          {t("book.delete")}
        </button>
      </section>
    </>
  );
}
