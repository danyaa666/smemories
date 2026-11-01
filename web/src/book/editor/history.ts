// Undo and redo as a snapshot stack of references (ADR 0004 Q4): the documents are immutable, so the old object is the snapshot (0.6 MB and
// 4 us per gesture at depth 100 on a 129 KB document). One step per user action; a gesture is begin(), many preview()s, one commit().
export const HISTORY_DEPTH = 100;

export class History<T> {
  present: T;
  private past: T[] = [];
  private future: T[] = [];
  private base: T | undefined; // the document before the gesture in progress
  private gesture = false;

  constructor(initial: T) {
    this.present = initial;
  }

  get canUndo() {
    return this.past.length > 0;
  }
  get canRedo() {
    return this.future.length > 0;
  }
  /** The last committed document: what autosave sends (a preview in progress is not saved). */
  get committed(): T {
    return this.gesture ? (this.base as T) : this.present;
  }
  get inGesture() {
    return this.gesture;
  }

  /** Starts a gesture (pointer down, first key of a burst). A gesture already in progress is committed first. */
  begin() {
    if (this.gesture) this.commit();
    this.base = this.present;
    this.gesture = true;
  }

  /** Shows `next` without a step (render only). Outside a gesture it starts one. */
  preview(next: T) {
    if (!this.gesture) this.begin();
    this.present = next;
  }

  /** Ends the gesture as one step. True when the document changed (so the caller marks it dirty). */
  commit(): boolean {
    if (!this.gesture) return false;
    this.gesture = false;
    const base = this.base as T;
    this.base = undefined;
    if (base === this.present) return false;
    this.past.push(base);
    if (this.past.length > HISTORY_DEPTH) this.past.shift();
    this.future = []; // a new step after undo drops the redo tail
    return true;
  }

  /** Throws the gesture away (Escape). */
  cancel() {
    if (!this.gesture) return;
    this.present = this.base as T;
    this.gesture = false;
    this.base = undefined;
  }

  /** One user action that is not a gesture. */
  push(next: T): boolean {
    this.begin();
    this.preview(next);
    return this.commit();
  }

  undo(): boolean {
    this.cancel();
    const prev = this.past.pop();
    if (prev === undefined) return false;
    this.future.push(this.present);
    this.present = prev;
    return true;
  }

  redo(): boolean {
    this.cancel();
    const next = this.future.pop();
    if (next === undefined) return false;
    this.past.push(this.present);
    this.present = next;
    return true;
  }

  /** Replaces the document and forgets every step (reset, "use theirs", a first load). */
  clear(doc: T) {
    this.present = doc;
    this.past = [];
    this.future = [];
    this.base = undefined;
    this.gesture = false;
  }
}
