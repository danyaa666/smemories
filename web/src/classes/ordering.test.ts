import { describe, expect, it } from "vitest";
import { arrayMove, orderPayload, reconcile, sameOrder, tabsToSpaces } from "./ordering";

const o = (members: string[], front: string[] = [], back: string[] = []) => ({
  members,
  front,
  back,
});

describe("ordering", () => {
  it("moves one item", () => {
    expect(arrayMove(["a", "b", "c"], 2, 0)).toEqual(["c", "a", "b"]);
    expect(arrayMove(["a", "b", "c"], 0, 1)).toEqual(["b", "a", "c"]);
  });

  it("builds the full-list payload the API demands", () => {
    expect(orderPayload("C1", o(["m2", "m1"], ["p1"], ["p3", "p2"]))).toEqual({
      class_id: "C1",
      member_ids: ["m2", "m1"],
      front_page_ids: ["p1"],
      back_page_ids: ["p3", "p2"],
    });
  });

  it("keeps the admin's arrangement over a fresh server list", () => {
    const local = o(["m2", "m1", "m3"], ["p1", "p2"]);
    // m3 left, m4 joined, p2 was deleted, p9 was added
    const server = o(["m1", "m2", "m4"], ["p1", "p9"]);
    expect(reconcile(local, server)).toEqual(o(["m2", "m1", "m4"], ["p1", "p9"]));
    expect(reconcile(null, server)).toBe(server);
  });

  it("compares orders", () => {
    expect(sameOrder(o(["a", "b"]), o(["a", "b"]))).toBe(true);
    expect(sameOrder(o(["a", "b"]), o(["b", "a"]))).toBe(false);
    expect(sameOrder(o(["a"], ["x"]), o(["a"]))).toBe(false);
  });

  it("turns every tab into two spaces", () => {
    expect(tabsToSpaces("a\tb\t\tc")).toBe("a  b    c");
  });
});
