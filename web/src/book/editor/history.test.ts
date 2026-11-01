import { describe, expect, it } from "vitest";
import { History, HISTORY_DEPTH } from "./history";

describe("History", () => {
  it("undo, redo, and a new step after undo drops the redo tail", () => {
    const h = new History(0);
    h.push(1);
    h.push(2);
    expect([h.canUndo, h.canRedo]).toEqual([true, false]);
    h.undo();
    expect([h.present, h.canRedo]).toEqual([1, true]);
    h.redo();
    expect(h.present).toBe(2);
    h.undo();
    h.undo();
    expect(h.present).toBe(0);
    expect(h.undo()).toBe(false);
    h.push(9);
    expect(h.canRedo).toBe(false);
    expect(h.redo()).toBe(false);
    h.undo();
    expect(h.present).toBe(0);
  });
  it("a gesture is one step: begin, many previews, one commit", () => {
    const h = new History(0);
    h.begin();
    for (let i = 1; i <= 50; i++) h.preview(i);
    expect(h.present).toBe(50);
    expect(h.committed).toBe(0); // autosave would not send the preview
    expect(h.canUndo).toBe(false);
    expect(h.commit()).toBe(true);
    expect(h.committed).toBe(50);
    h.undo();
    expect(h.present).toBe(0);
    expect(h.canUndo).toBe(false);
  });
  it("a gesture that ends where it began is no step; cancel restores", () => {
    const h = new History(5);
    h.begin();
    h.preview(6);
    h.preview(5);
    expect(h.commit()).toBe(false);
    expect(h.canUndo).toBe(false);
    h.begin();
    h.preview(7);
    h.cancel();
    expect(h.present).toBe(5);
  });
  it("undo during a gesture throws the gesture away first", () => {
    const h = new History(0);
    h.push(1);
    h.begin();
    h.preview(2);
    h.undo();
    expect(h.present).toBe(0);
    expect(h.inGesture).toBe(false);
  });
  it(`keeps ${HISTORY_DEPTH} steps and drops the oldest`, () => {
    const h = new History(0);
    for (let i = 1; i <= 150; i++) h.push(i);
    let n = 0;
    while (h.undo()) n++;
    expect(n).toBe(HISTORY_DEPTH);
    expect(h.present).toBe(50); // 150 - 100
  });
  it("clear forgets everything", () => {
    const h = new History(0);
    h.push(1);
    h.undo();
    h.clear(7);
    expect([h.present, h.canUndo, h.canRedo]).toEqual([7, false, false]);
  });
});
