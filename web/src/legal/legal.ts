// The Terms of Use and the Privacy Policy, read at build time from docs/legal (the one source of truth, D-35; vite.config.ts
// allows the import, `?raw` gives the file as a string). The files use only a front matter, headings, paragraphs, "- " bullet
// lists and **bold**, which is all parseLegal understands.
import privacyEn from "../../../docs/legal/privacy.en.md?raw";
import privacyVi from "../../../docs/legal/privacy.vi.md?raw";
import termsEn from "../../../docs/legal/terms.en.md?raw";
import termsVi from "../../../docs/legal/terms.vi.md?raw";

export type Block =
  | { kind: "heading"; level: 1 | 2 | 3; text: string }
  | { kind: "paragraph"; text: string }
  | { kind: "list"; items: string[] };

export type LegalDoc = {
  version: string;
  status: string;
  /** The front matter status starts with DRAFT (English) or BẢN NHÁP (Vietnamese): not the final text yet. */
  draft: boolean;
  blocks: Block[];
};

export type DocName = "terms" | "privacy";
export type Lang = "en" | "vi";

export function parseLegal(raw: string): LegalDoc {
  const fm = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/.exec(raw);
  const meta: Record<string, string> = {};
  for (const line of (fm?.[1] ?? "").split(/\r?\n/)) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  const blocks: Block[] = [];
  let para: string[] = [];
  let list: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ kind: "paragraph", text: para.join(" ") });
    if (list.length) blocks.push({ kind: "list", items: list });
    para = [];
    list = [];
  };
  for (const line of raw.slice(fm?.[0].length ?? 0).split(/\r?\n/)) {
    const l = line.trim();
    const heading = /^(#{1,3})\s+(.+)$/.exec(l);
    const bullet = /^[-*]\s+(.+)$/.exec(l);
    if (!l) flush();
    else if (heading?.[1] && heading[2]) {
      flush();
      blocks.push({
        kind: "heading",
        level: heading[1].length as 1 | 2 | 3,
        text: heading[2],
      });
    } else if (bullet?.[1]) {
      if (para.length) flush();
      list.push(bullet[1]);
    } else if (list.length)
      list[list.length - 1] += ` ${l}`; // a wrapped bullet
    else para.push(l);
  }
  flush();
  const status = meta.status ?? "";
  return {
    version: meta.version ?? "",
    status,
    draft: /^(DRAFT|BẢN NHÁP)/i.test(status),
    blocks,
  };
}

const DOCS: Record<DocName, Record<Lang, LegalDoc>> = {
  terms: { en: parseLegal(termsEn), vi: parseLegal(termsVi) },
  privacy: { en: parseLegal(privacyEn), vi: parseLegal(privacyVi) },
};

export const legalDoc = (name: DocName, lang: Lang): LegalDoc => DOCS[name][lang];

/** The version the sign-up and accept screens send; the API refuses any other (internal/legal/legal.go, tested to agree). */
export const TERMS_VERSION = DOCS.terms.en.version;
