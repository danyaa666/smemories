// Stand-in for the book (the real one is web/src/book): a text the student edits, saved by an autosave timer. The failure tests check that it is never touched by the studio.
import { useEffect, useRef, useState } from "react";
export function Book({ onSaved }) {
  const [text, setText] = useState("Lớp 12A1");
  const dirty = useRef(false);
  const saves = useRef(0);
  useEffect(() => {
    const t = setInterval(() => { if (dirty.current) { dirty.current = false; saves.current++; window.__book = { text: document.querySelector("#book-text").value, saves: saves.current }; onSaved?.(); } }, 100);
    return () => clearInterval(t);
  }, [onSaved]);
  return <textarea id="book-text" value={text} onChange={(e) => { setText(e.target.value); dirty.current = true; }} aria-label="book text" />;
}
