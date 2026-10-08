import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { createYearbook } from "../api/yearbooks";
import { BookForm } from "../components/BookForm";
import { YEARBOOKS_KEY } from "./Yearbooks";

export function YearbookNew() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const m = useMutation({
    mutationFn: createYearbook,
    onSuccess: (y) => {
      qc.setQueryData(["yearbook", y.id], y);
      void qc.invalidateQueries({ queryKey: YEARBOOKS_KEY });
      void navigate(`/yearbooks/${y.id}`); // next: the profile and the photos
    },
  });
  return (
    <>
      <h1>{t("books.newTitle")}</h1>
      <BookForm
        initial={{
          title: "",
          school_name: "",
          class_name: "",
          graduation_year: null,
          motto: "",
          language: i18n.language.startsWith("vi") ? "vi" : "en",
          page_size: "A5",
        }}
        submitLabel={t("books.create")}
        pending={m.isPending}
        error={m.error}
        onSubmit={(b) => m.mutate(b)}
      />
    </>
  );
}
