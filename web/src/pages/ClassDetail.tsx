import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router-dom";
import { getClass, leaveClass } from "../api/classes";
import { ApiError } from "../api/client";
import { errorText } from "../auth/errors";
import { classErrorText } from "../classes/classErrors";
import { Invites } from "../classes/Invites";
import { MyPage } from "../classes/MyPage";
import { Roster } from "../classes/Roster";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { CLASSES_KEY } from "./Classes";

/** /classes/:id. Everyone sees the class and their own page; the roster and the invites are mounted for admins only. */
export function ClassDetail() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const q = useQuery({ queryKey: ["class", id], queryFn: () => getClass(id) });
  const [leaving, setLeaving] = useState(false);
  const leave = useMutation({
    mutationFn: () => leaveClass(id),
    onSuccess: () => {
      qc.removeQueries({ queryKey: ["class", id] });
      void qc.invalidateQueries({ queryKey: CLASSES_KEY });
      void navigate("/classes");
    },
  });
  if (q.isPending) return <p role="status">{t("auth.loading")}</p>;
  if (q.isError) {
    const gone = q.error instanceof ApiError && (q.error.status === 404 || q.error.status === 400);
    return (
      <>
        <p role="alert" className="error">
          {gone ? t("classes.notFound") : errorText(t, q.error)}
        </p>
        {!gone && <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>}
        <p>
          <Link to="/classes">{t("classes.back")}</Link>
        </p>
      </>
    );
  }
  const c = q.data;
  const meta = [c.school_name, c.graduation_year].filter(Boolean).join(" · ");
  return (
    <>
      <p>
        <Link to="/classes">{t("classes.back")}</Link>
      </p>
      <h1>{c.name}</h1>
      {meta && <p>{meta}</p>}
      <p>
        {t("classes.yourRole")}: <strong>{t(`classes.role.${c.my_role}`)}</strong>
      </p>
      <MyPage cls={c} />
      {c.my_role === "admin" && (
        <>
          <Invites classId={c.id} />
          <Roster classId={c.id} />
        </>
      )}
      <section aria-labelledby="leave">
        <h2 id="leave">{t("classes.leave.title")}</h2>
        <Button
          variant="danger"
          onClick={() => {
            leave.reset();
            setLeaving(true);
          }}
        >
          {t("classes.leave.button")}
        </Button>
      </section>
      {leaving && (
        <Dialog
          title={t("classes.leave.confirmTitle", { name: c.name })}
          confirmLabel={t("classes.leave.button")}
          cancelLabel={t("classes.cancel")}
          danger
          busy={leave.isPending}
          onConfirm={() => leave.mutate()}
          onCancel={() => setLeaving(false)}
        >
          <p>{t("classes.leave.body")}</p>
          {leave.error && (
            <p role="alert" className="error">
              {classErrorText(t, leave.error, "leave")}
            </p>
          )}
        </Dialog>
      )}
    </>
  );
}
