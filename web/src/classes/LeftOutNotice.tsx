import { useTranslation } from "react-i18next";
import type { ClassBook } from "../api/classBook";
import { Banner } from "../components/ui/Banner";

/** Students whose notes were left out so that the answer stays under 8 MiB (`truncated` of class-book/get, T-133). */
export const leftOut = (book: ClassBook) => {
  const ids = new Set(book.truncated ?? []);
  return book.members.filter((m) => ids.has(m.member_id));
};

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
  if (students.length === 0) return null;
  return (
    <Banner variant="warning">
      <span>
        {t("classBook.leftOut", {
          count: students.length,
          names: students.map((m) => m.name.trim() || t("classBook.unnamed")).join(", "),
        })}
      </span>
      {onAck && (
        <label className="legal-consent">
          <input type="checkbox" checked={acked} onChange={(e) => onAck(e.target.checked)} />{" "}
          {t("classBook.leftOutAck")}
        </label>
      )}
    </Banner>
  );
}
