// The keyboard shortcuts of the layout editor (T-160), one table for the help sheet and the handler. Chosen so that none is taken by the browser or the
// system on Windows, Linux or macOS: plain Arrow and Shift+Arrow are free on a focused button; Alt+Arrow is browser Back/Forward on Windows and Linux
// (so Alt alone is never used), Ctrl+Arrow switches desktops on macOS and Ctrl+Alt+Arrow is a workspace or screen-rotation key on Linux and Windows
// (so no Ctrl); `[` `]` need AltGr on some layouts. Alt+Shift+Arrow is free in the browsers; on Windows with two input languages Alt+Shift alone
// switches the language, but not with an Arrow pressed in between: listed as a manual check (docs/e2e.md).
import type { Nudge } from "./math";

export const ARROWS: Record<string, readonly [number, number]> = {
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
};
/** `,` and `.` by their character (US layout: Shift gives `<` and `>`); `-1` turns left. */
export const ROTATE_KEYS: Record<string, -1 | 1> = { ",": -1, "<": -1, ".": 1, ">": 1 };

export const STEP = 1; // design px
export const BIG_STEP = 10; // Shift+Arrow and the resize keys
export const TURN = 1; // degrees
export const BIG_TURN = 15; // Shift + , and .

/** What a key event asks for, or null when it is not ours (the browser keeps Alt+Arrow, Ctrl and Cmd combinations). */
export function nudgeOf(e: {
  key: string;
  shiftKey: boolean;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
}): Nudge | null {
  if (e.ctrlKey || e.metaKey) return null;
  const a = ARROWS[e.key];
  if (a) {
    if (e.altKey && e.shiftKey) return { kind: "resize", dw: a[0] * BIG_STEP, dh: a[1] * BIG_STEP };
    if (e.altKey) return null;
    const s = e.shiftKey ? BIG_STEP : STEP;
    return { kind: "move", dx: a[0] * s, dy: a[1] * s };
  }
  const r = ROTATE_KEYS[e.key];
  if (r && !e.altKey) return { kind: "rotate", ddeg: r * (e.shiftKey ? BIG_TURN : TURN) };
  return null;
}

/** The rows of the help sheet: an id (the description is `editor.help.<id>`) and the keys as printed. */
export const SHORTCUTS: readonly { id: string; keys: string }[] = [
  { id: "tab", keys: "Tab, Shift+Tab" },
  { id: "move", keys: "← ↑ → ↓" },
  { id: "moveBig", keys: "Shift + ← ↑ → ↓" },
  { id: "resize", keys: "Alt + Shift + ← ↑ → ↓ (Mac: Option + Shift)" },
  { id: "rotate", keys: ", and ." },
  { id: "rotateBig", keys: "Shift + , and Shift + ." },
  { id: "delete", keys: "Delete, Backspace" },
  { id: "escape", keys: "Esc" },
  { id: "undo", keys: "Ctrl + Z (Mac: Cmd + Z)" },
  { id: "redo", keys: "Ctrl + Shift + Z, Ctrl + Y (Mac: Cmd + Shift + Z)" },
  { id: "noSnap", keys: "Alt (Mac: Option) while dragging" },
  { id: "ratio", keys: "Shift while resizing" },
  { id: "snap15", keys: "Shift while rotating" },
  { id: "help", keys: "?" },
];
