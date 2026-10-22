// T-116: the server lets one account download 64 photos at once (T-107) and answers the 65th with 429, which an <img> never retries.
// Every gallery image goes through this queue, so a page of 100 photos asks for at most LIMIT at a time, far below that cap.
export const LIMIT = 16;
const SLOT_TIMEOUT_MS = 30_000; // ponytail: an image that never fires load/error frees its slot after this; a smarter watchdog if it matters

const waiting: (() => void)[] = [];
let active = 0;
/** URLs that finished loading: the browser has them cached, so they need no slot. */
export const loaded = new Set<string>();

function pump() {
  while (active < LIMIT && waiting.length) {
    active++;
    waiting.shift()!();
  }
}

/**
 * Calls `start(done)` when a slot is free; the caller calls `done()` when the image has loaded or failed.
 * The returned function cancels (leaves the line, or frees the slot).
 */
export function enqueue(start: (done: () => void) => void): () => void {
  let state: "waiting" | "running" | "done" = "waiting";
  let timer: ReturnType<typeof setTimeout> | undefined;
  const done = () => {
    if (state !== "running") return;
    state = "done";
    clearTimeout(timer);
    active--;
    pump();
  };
  const job = () => {
    state = "running";
    timer = setTimeout(done, SLOT_TIMEOUT_MS);
    start(done);
  };
  waiting.push(job);
  pump();
  return () => {
    if (state === "waiting") {
      waiting.splice(waiting.indexOf(job), 1);
      state = "done";
    } else done();
  };
}

/** Test hook: slots in use and people waiting. */
export const queueStats = () => ({ active, waiting: waiting.length });
