import { useTranslation } from "react-i18next";
import type { ClassBook } from "../api/classBook";
import { Banner } from "../components/ui/Banner";

/** Students whose notes were left out so that the answer stays under 8 MiB (`truncated` of class-book/get, T-133). */
export const leftOut = (book: ClassBook) => {
  const ids = new Set(book.truncated ?? []);
  return book.members.filter((m) => ids.has(m.member_id));
};

/** Students whose free layout was left out for the same reason (`truncated_layouts`, T-167): their notes and profile are still printed. */
export const layoutLeftOut = (book: ClassBook) => {
  const ids = new Set(book.truncated_layouts ?? []);
  return book.members.filter((m) => ids.has(m.member_id));
};

/** The ids of both lists, one key: the admin's yes is asked again when either changes. */
export const leftOutKey = (book: ClassBook) =>
  [...leftOut(book), ...layoutLeftOut(book)].map((m) => m.member_id).join(",");

/** The warning, by name. With `onAck` it also asks the admin to confirm before the book is printed. */
export function LeftOutNotice({
  book,
  acked,
  onAck,
}: {
  book: ClassBook;
  acked?: boolean;
  onAck?: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  const students = leftOut(book);
  const layouts = layoutLeftOut(book);
  if (students.length === 0 && layouts.length === 0) return null;
  const names = (ms: typeof students) =>
    ms.map((m) => m.name.trim() || t("classBook.unnamed")).join(", ");
  const ack =
    students.length > 0 && layouts.length > 0
      ? "classBook.leftOutAckBoth"
      : students.length > 0
        ? "classBook.leftOutAck"
        : "classBook.layoutLeftOutAck";
  return (
    <>
      {students.length > 0 && (
        <Banner variant="warning">
          <span>{t("classBook.leftOut", { count: students.length, names: names(students) })}</span>
        </Banner>
      )}
      {layouts.length > 0 && (
        <Banner variant="warning">
          <span>
            {t("classBook.layoutLeftOut", { count: layouts.length, names: names(layouts) })}
          </span>
        </Banner>
      )}
      {onAck && (
        <label className="legal-consent">
          <input type="checkbox" checked={acked} onChange={(e) => onAck(e.target.checked)} />{" "}
          {t(ack)}
        </label>
      )}
    </>
  );
}
