import type { TFunction } from "i18next";
import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { ClassBook, ClassBookMember, ClassBookPage } from "../api/classBook";
import { ProfilePages } from "./BookPages";
import { COVER_KEY, memberBook, memberPrefix, pageKey } from "./classBook";
import { BookEnvProvider, Sheet, useBookEnv } from "./env";
import { FitScope, FitText } from "./fit";
import type { PageSize } from "./sizes";

type Env = { size: PageSize; eager: boolean; track: (delta: number) => void };

function Cover({ cls }: { cls: ClassBook["class"] }) {
  const { t } = useBookEnv();
  return (
    <Sheet label="class-cover">
      <div className="bk-cover bk-cover-plain">
        <FitText fitKey={COVER_KEY} className="bk-title" style={{ fontSize: 56 }}>
          {cls.name}
        </FitText>
        {cls.school_name && <p className="bk-sub">{cls.school_name}</p>}
        {cls.graduation_year !== null && (
          <p className="bk-sub">{t("bookPages.classOf", { year: cls.graduation_year })}</p>
        )}
      </div>
    </Sheet>
  );
}

/** A class page (intro, teachers, ...): a title and a body of plain text, newlines kept, shrunk to fit one page. Always React text, never markup. */
function ClassPage({ page }: { page: ClassBookPage }) {
  return (
    <Sheet label="class-page">
      <div className="bk-profile">
        <h2 className="bk-cp-title">{page.title}</h2>
        <FitText fitKey={pageKey(page.id)} className="bk-box-text" style={{ fontSize: 20 }}>
          {page.body}
        </FitText>
      </div>
    </Sheet>
  );
}

function BackCover({ cls }: { cls: ClassBook["class"] }) {
  const { t } = useBookEnv();
  return (
    <Sheet label="class-back">
      <div className="bk-back">
        <p className="bk-title-small">{cls.name}</p>
        <p className="bk-sub">{t("bookPages.thanks")}</p>
      </div>
    </Sheet>
  );
}

/** One student's profile and notes pages in their own template and language. Nothing is drawn for a student without a page. */
const StudentPages = memo(function StudentPages({
  cls,
  member,
  env,
}: {
  cls: ClassBook["class"];
  member: ClassBookMember;
  env: Env;
}) {
  const { i18n } = useTranslation();
  const data = useMemo(() => memberBook(cls, member, env.size), [cls, member, env.size]);
  const lang = data?.yearbook.language;
  const t = useMemo(() => i18n.getFixedT(lang ?? i18n.language), [i18n, lang]);
  const value = useMemo(() => ({ ...env, t }), [env, t]);
  if (!data) return null;
  return (
    <BookEnvProvider value={value}>
      <FitScope prefix={memberPrefix(member)}>
        <ProfilePages data={data} />
      </FitScope>
    </BookEnvProvider>
  );
});

/**
 * The class book (T-130): the cover, the front class pages, then for each student in order the existing profile and notes pages of
 * their template, the back class pages and the back cover. The cover and class pages are in the class's language, a student's pages in theirs.
 * Memoised like BookPages: fit and photo progress do not re-render the pages.
 */
export const ClassBookPages = memo(function ClassBookPages({
  book,
  size,
  t,
  eager,
  track,
}: {
  book: ClassBook;
  size: PageSize;
  t: TFunction;
  eager: boolean;
  track: (delta: number) => void;
}) {
  const env = useMemo(() => ({ size, eager, track }), [size, eager, track]);
  const classEnv = useMemo(() => ({ ...env, t }), [env, t]);
  return (
    <>
      <BookEnvProvider value={classEnv}>
        <Cover cls={book.class} />
        {book.front_pages.map((p) => (
          <ClassPage key={p.id} page={p} />
        ))}
      </BookEnvProvider>
      {book.members.map((m) => (
        <StudentPages key={m.member_id} cls={book.class} member={m} env={env} />
      ))}
      <BookEnvProvider value={classEnv}>
        {book.back_pages.map((p) => (
          <ClassPage key={p.id} page={p} />
        ))}
        <BackCover cls={book.class} />
      </BookEnvProvider>
    </>
  );
});
