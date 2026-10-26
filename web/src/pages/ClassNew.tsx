import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { createClass } from "../api/classes";
import { ClassForm } from "../classes/ClassForm";
import { CLASSES_KEY } from "./Classes";

export function ClassNew() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const m = useMutation({
    mutationFn: createClass,
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: CLASSES_KEY });
      void navigate(`/classes/${c.id}`);
    },
  });
  return (
    <>
      <h1>{t("classes.newTitle")}</h1>
      <ClassForm
        initial={{ name: "", language: i18n.language.startsWith("vi") ? "vi" : "en" }}
        ctx="create"
        submitLabel={t("classes.form.submit")}
        pending={m.isPending}
        error={m.error}
        onSubmit={m.mutate}
      />
    </>
  );
}
