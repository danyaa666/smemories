import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Banner } from "../components/ui/Banner";
import { legalDoc, type Block, type DocName, type LegalDoc } from "./legal";

/** **bold** to <strong>; everything else is text, which React escapes. */
function inline(text: string): ReactNode[] {
  return text
    .split(/\*\*(.+?)\*\*/)
    .map((part, i) =>
      i % 2 ? <strong key={i}>{part}</strong> : <Fragment key={i}>{part}</Fragment>,
    );
}

export function LegalBlocks({ blocks }: { blocks: Block[] }) {
  return blocks.map((b, i) => {
    switch (b.kind) {
      case "heading": {
        const H = `h${b.level}` as "h1" | "h2" | "h3";
        return <H key={i}>{inline(b.text)}</H>;
      }
      case "list":
        return (
          <ul key={i}>
            {b.items.map((item, j) => (
              <li key={j}>{inline(item)}</li>
            ))}
          </ul>
        );
      case "paragraph":
        return <p key={i}>{inline(b.text)}</p>;
    }
  });
}

export function LegalView({ doc }: { doc: LegalDoc }) {
  const { t } = useTranslation();
  return (
    <article className="legal-doc">
      {doc.draft && <Banner variant="warning">{t("legal.draft")}</Banner>}
      <LegalBlocks blocks={doc.blocks} />
    </article>
  );
}

/** /terms and /privacy: the document in the UI language, with a banner while it is a draft. */
export function LegalPage({ doc }: { doc: DocName }) {
  const { i18n } = useTranslation();
  return <LegalView doc={legalDoc(doc, i18n.resolvedLanguage === "vi" ? "vi" : "en")} />;
}
