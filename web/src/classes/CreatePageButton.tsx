import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { setBook, type SchoolClass } from "../api/classes";
import { createYearbook } from "../api/yearbooks";
import { Button } from "../components/ui/Button";
import { YEARBOOKS_KEY } from "../pages/Yearbooks";
import { classErrorText } from "./classErrors";

/**
 * "Create my page": a new yearbook prefilled from the class, linked as my page, then the yearbook editor.
 * If the link step fails after the book exists, a retry only repeats the link: no second book.
 */
export function CreatePageButton({ cls }: { cls: SchoolClass }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const made = useRef<string | undefined>(undefined);
  const m = useMutation({
    mutationFn: async () => {
      if (!made.current) {
        const y = await createYearbook({
          title: cls.name,
          school_name: cls.school_name,
          class_name: cls.name,
          graduation_year: cls.graduation_year,
          motto: "",
          language: cls.language,
          page_size: "A5",
        });
        made.current = y.id;
      }
      await setBook(cls.id, made.current);
      return made.current;
    },
    onSuccess: (bookId) => {
      void qc.invalidateQueries({ queryKey: ["class", cls.id] });
      void qc.invalidateQueries({ queryKey: YEARBOOKS_KEY });
      void navigate(`/yearbooks/${bookId}`);
    },
  });
  return (
    <>
      {m.error && (
        <p role="alert" className="error">
          {classErrorText(t, m.error, "page")}
        </p>
      )}
      <Button variant="primary" busy={m.isPending} onClick={() => m.mutate()}>
        {m.isPending ? t("auth.working") : t("classes.page.create")}
      </Button>
    </>
  );
}
