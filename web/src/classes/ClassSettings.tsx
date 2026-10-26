import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { deleteClass, updateClass, type SchoolClass } from "../api/classes";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { CLASSES_KEY } from "../pages/Classes";
import { ClassForm } from "./ClassForm";
import { classErrorText } from "./classErrors";

/** Admin only: rename the class and delete it. Mounted for admins alone, so a student never sees or calls either. */
export function ClassSettings({ cls }: { cls: SchoolClass }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [confirming, setConfirming] = useState(false);
  const [saved, setSaved] = useState(false);
  const save = useMutation({
    mutationFn: (body: Parameters<typeof updateClass>[1]) => updateClass(cls.id, body),
    onSuccess: (c) => {
      qc.setQueryData(["class", cls.id], c);
      void qc.invalidateQueries({ queryKey: CLASSES_KEY });
      setSaved(true);
    },
  });
  const del = useMutation({
    mutationFn: () => deleteClass(cls.id),
    onSuccess: () => {
      qc.removeQueries({ queryKey: ["class", cls.id] });
      void qc.invalidateQueries({ queryKey: CLASSES_KEY });
      void navigate("/classes");
    },
  });
  return (
    <>
      <section aria-labelledby="class-edit">
        <h2 id="class-edit">{t("classes.edit.title")}</h2>
        <ClassForm
          // a saved class changes `initial`; the key re-seeds the fields from the server's copy
          key={cls.updated_at}
          initial={{
            name: cls.name,
            school_name: cls.school_name,
            graduation_year: cls.graduation_year,
            language: cls.language,
          }}
          ctx="edit"
          submitLabel={t("classes.edit.submit")}
          pending={save.isPending}
          error={save.error}
          onSubmit={(b) => {
            setSaved(false);
            save.mutate(b);
          }}
        />
        {saved && !save.error && <p role="status">{t("classes.edit.saved")}</p>}
      </section>
      <section aria-labelledby="class-delete">
        <h2 id="class-delete">{t("classes.delete.title")}</h2>
        <Button
          variant="danger"
          onClick={() => {
            del.reset();
            setConfirming(true);
          }}
        >
          {t("classes.delete.button")}
        </Button>
      </section>
      {confirming && (
        <Dialog
          title={t("classes.delete.confirmTitle", { name: cls.name })}
          confirmLabel={t("classes.delete.button")}
          cancelLabel={t("classes.cancel")}
          danger
          busy={del.isPending}
          onConfirm={() => del.mutate()}
          onCancel={() => setConfirming(false)}
        >
          <p>{t("classes.delete.body")}</p>
          {del.error && (
            <p role="alert" className="error">
              {classErrorText(t, del.error, "delete")}
            </p>
          )}
        </Dialog>
      )}
    </>
  );
}
