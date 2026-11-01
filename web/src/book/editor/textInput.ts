// What a text box lets in (T-161, ADR 0004 Q3). Pure: the rules of the 500-rune cap and of pasted text, shared by the field on the page and the
// field in the property panel. The count is document.ts `runeCount` (code points after NFC and trim, as the server), never `maxlength` (UTF-16 units).
import { MAX_TEXT, runeCount } from "./document";

/**
 * The longest start of `text` that is within the cap (cut at a code point, never inside a surrogate pair); `text` itself when it already is.
 * A text of decomposed letters counts as its composed form, so it is cut only when that is over 500.
 */
export function capText(text: string): string {
  if (runeCount(text) <= MAX_TEXT) return text;
  const cps = [...text];
  let lo = 0; // the most code points known to fit
  let hi = cps.length; // known not to fit
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (runeCount(cps.slice(0, mid).join("")) <= MAX_TEXT) lo = mid;
    else hi = mid;
  }
  return cps.slice(0, lo).join("");
}

// What the server refuses and the field has no use for: Cc except the newline (a tab becomes a space), Cf except the joiner, Zl, Zp, U+FFFD, lone surrogates.
const REFUSED = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\p{Cs}�]/gu;

/** Plain text only: line breaks as `\n`, a tab as one space, and none of the characters the server refuses. */
export function plainText(raw: string): string {
  return raw
    .replace(/\r\n?/g, "\n")
    .replace(/\t/g, " ")
    .replace(REFUSED, (c) => (c === "\n" || c === "‍" ? c : ""));
}

/** What a field keeps of a value the browser just produced: plain, and within the cap. */
export const fixText = (raw: string) => capText(plainText(raw));
