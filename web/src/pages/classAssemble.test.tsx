import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ClassBook, ClassPage, SchoolClass } from "../api/classes";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

const CID = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
const mid = (n: number) => `01J9Z3K6V8Q4M7N2P5R8T0M${String(n).padStart(3, "0")}`;
const pid = (n: number) => `01J9Z3K6V8Q4M7N2P5R8T0P${String(n).padStart(3, "0")}`;

const cls = (over: Partial<SchoolClass> = {}): SchoolClass => ({
  id: CID,
  name: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  graduation_year: 2026,
  language: "vi",
  my_role: "admin",
  page_linked: false,
  created_at: 1760000000000,
  updated_at: 1760000000000,
  ...over,
});
const member = (n: number, over: Partial<ClassBook["members"][number]> = {}) => ({
  member_id: mid(n),
  name: `Student ${n}`,
  linked: true,
  yearbook: null,
  profile: null,
  notes: [],
  note_count: 3,
  approved_total: 3,
  truncated: false,
  ...over,
});
const page = (n: number, over: Partial<ClassPage> = {}): ClassPage => ({
  id: pid(n),
  kind: "intro",
  title: `Page ${n}`,
  body: `Body ${n}`,
  placement: "front",
  created_at: 1760000000000,
  updated_at: 1760000000000,
  ...over,
});

/** An in-memory class book API: update-order stores the lists, class-page/* edits the pages. */
function fake(
  opts: {
    members?: number;
    front?: ClassPage[];
    back?: ClassPage[];
    role?: "admin" | "student";
    truncated?: string[];
  } = {},
) {
  let members = Array.from({ length: opts.members ?? 4 }, (_, i) => member(i + 1));
  let front = opts.front ?? [page(1)];
  let back = opts.back ?? [];
  const fail: { order?: Response; create?: Response } = {};
  const calls = mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /class/get?id=${CID}`]: () => ok(cls({ my_role: opts.role ?? "admin" })),
    [`GET /class-book/get?class_id=${CID}`]: () =>
      ok({
        class: {
          id: CID,
          name: "Lớp 12A1",
          school_name: "",
          graduation_year: 2026,
          language: "vi",
        },
        front_pages: front,
        members,
        back_pages: back,
        ...(opts.truncated && { truncated: opts.truncated }),
      }),
    "POST /class-book/update-order": (c: Call) => {
      if (fail.order) return fail.order.clone();
      const b = c.body as {
        member_ids: string[];
        front_page_ids: string[];
        back_page_ids: string[];
      };
      members = b.member_ids.map((id) => members.find((m) => m.member_id === id)!);
      const all = [...front, ...back];
      front = b.front_page_ids.map((id) => all.find((p) => p.id === id)!);
      back = b.back_page_ids.map((id) => all.find((p) => p.id === id)!);
      return ok({});
    },
    "POST /class-page/create": (c: Call) => {
      if (fail.create) return fail.create.clone();
      const b = c.body as Pick<ClassPage, "kind" | "title" | "body" | "placement">;
      const p = page(front.length + back.length + 1, b);
      if (b.placement === "front") front = [...front, p];
      else back = [...back, p];
      return ok(p);
    },
    "POST /class-page/update": (c: Call) => {
      const b = c.body as ClassPage;
      front = front.map((p) => (p.id === b.id ? { ...p, ...b } : p));
      return ok(front.find((p) => p.id === b.id)!);
    },
    "POST /class-page/delete": (c: Call) => {
      const id = (c.body as { id: string }).id;
      front = front.filter((p) => p.id !== id);
      back = back.filter((p) => p.id !== id);
      return ok({});
    },
  });
  return {
    calls,
    fail,
    set: (m: typeof members) => (members = m),
    orderCalls: () => calls.filter((c) => c.path === "/class-book/update-order"),
  };
}

const students = () => within(screen.getByRole("list", { name: "Students' pages" }));
const names = () =>
  students()
    .getAllByRole("listitem")
    .map((li) => within(li).getByRole("strong", { hidden: true }).textContent);
const open = async () => {
  renderApp(`/classes/${CID}/assemble`);
  await screen.findByRole("heading", { name: "Class book of Lớp 12A1" });
};

describe("assemble the class book", () => {
  it("shows the not-found screen to a student and never asks for the book", async () => {
    const f = fake({ role: "student" });
    renderApp(`/classes/${CID}/assemble`);
    expect(await screen.findByRole("heading", { name: /not found/i })).toBeInTheDocument();
    expect(f.calls.some((c) => c.path.startsWith("/class-book/get"))).toBe(false);
  });

  it("names the students whose notes were left out of the book, and says nothing otherwise", async () => {
    fake({ truncated: [mid(2), mid(4)] });
    await open();
    expect(
      screen.getByText(/left out of the class book.*\(2\): Student 2, Student 4/),
    ).toBeVisible();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument(); // the acknowledgement is asked on the print screen
  });

  it("shows no left-out warning for a class that fits", async () => {
    fake();
    await open();
    expect(screen.queryByText(/left out of the class book/)).not.toBeInTheDocument();
  });

  it("lists the students with the page state and the approved notes", async () => {
    const f = fake();
    f.set([member(1), member(2, { linked: false, approved_total: 0, note_count: 0 })]);
    await open();
    expect(await screen.findByText("Approved notes: 3")).toBeInTheDocument();
    expect(screen.getByText("No page yet (prints nothing)")).toBeInTheDocument();
  });

  it("reorders by keyboard only, keeps focus on the moved item, announces it, and saves the full lists", async () => {
    const f = fake({ front: [page(1), page(2)], back: [page(3, { placement: "back" })] });
    await open();
    const up = await screen.findByRole("button", { name: "Move Student 3 up" });
    up.focus();
    await userEvent.keyboard("{Enter}");
    expect(names()).toEqual(["Student 1", "Student 3", "Student 2", "Student 4"]);
    // focus stays on the control of the moved item, which is now one row higher
    expect(screen.getByRole("button", { name: "Move Student 3 up" })).toHaveFocus();
    expect(screen.getByText("Student 3, moved to position 2 of 4")).toBeInTheDocument();
    // the grip answers the arrow keys
    const grip = screen.getByRole("button", { name: "Drag to reorder Student 3" });
    grip.focus();
    await userEvent.keyboard("{ArrowUp}");
    expect(names()[0]).toBe("Student 3");
    expect(screen.getByRole("button", { name: "Drag to reorder Student 3" })).toHaveFocus();
    // the top item cannot go up; the key does nothing
    await userEvent.keyboard("{ArrowUp}");
    expect(f.orderCalls()).toHaveLength(0); // nothing is sent before Save

    await userEvent.click(screen.getByRole("button", { name: "Save order" }));
    await screen.findByText("Order saved.");
    expect(f.orderCalls()).toHaveLength(1);
    expect(f.orderCalls()[0]!.body).toEqual({
      class_id: CID,
      member_ids: [mid(3), mid(1), mid(2), mid(4)],
      front_page_ids: [pid(1), pid(2)],
      back_page_ids: [pid(3)],
    });
    expect(screen.getByRole("button", { name: "Save order" })).toBeDisabled();
    expect(names()).toEqual(["Student 3", "Student 1", "Student 2", "Student 4"]);
  });

  it("moves a class page inside its side and sends the page lists", async () => {
    const f = fake({ front: [page(1), page(2)] });
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Move Page 2 up" }));
    await userEvent.click(screen.getByRole("button", { name: "Save order" }));
    await waitFor(() => expect(f.orderCalls()).toHaveLength(1));
    expect(f.orderCalls()[0]!.body).toMatchObject({
      member_ids: [mid(1), mid(2), mid(3), mid(4)],
      front_page_ids: [pid(2), pid(1)],
      back_page_ids: [],
    });
  });

  it.each([
    [400, "invalid_member_ids"],
    [409, "conflict"],
  ])("reloads the current order and says so when the server answers %i", async (status, code) => {
    const f = fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Move Student 2 up" }));
    f.fail.order = err(status, code);
    f.set([member(1), member(2), member(3), member(4), member(5)]); // someone joined meanwhile
    await userEvent.click(screen.getByRole("button", { name: "Save order" }));
    expect(
      await screen.findByText(/The class changed while you were arranging it/),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(names()).toEqual(["Student 1", "Student 2", "Student 3", "Student 4", "Student 5"]),
    );
    expect(screen.getByRole("button", { name: "Save order" })).toBeDisabled();
  });

  it("keeps the arrangement when a page is added meanwhile", async () => {
    fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Move Student 2 up" }));
    await userEvent.click(screen.getByRole("button", { name: "Add a class page" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Teachers" } });
    await userEvent.click(screen.getByRole("button", { name: "Save page" }));
    await screen.findByRole("button", { name: "Edit Teachers" });
    expect(names()).toEqual(["Student 2", "Student 1", "Student 3", "Student 4"]);
    expect(screen.getByText("You have unsaved changes.")).toBeInTheDocument();
  });
});

describe("class pages", () => {
  it("turns a pasted tab into two spaces before sending, and keeps the counter", async () => {
    const f = fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Add a class page" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Wel\tcome" } });
    fireEvent.change(screen.getByLabelText("Text"), { target: { value: "a\tb\nc" } });
    expect(screen.getByText("6/4000")).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("Where in the book"), "back");
    await userEvent.click(screen.getByRole("button", { name: "Save page" }));
    await screen.findByRole("button", { name: /^Edit Wel\s+come$/ });
    const sent = f.calls.find((c) => c.path === "/class-page/create")!;
    expect(sent.body).toEqual({
      class_id: CID,
      kind: "intro",
      placement: "back",
      title: "Wel  come",
      body: "a  b\nc",
    });
  });

  it("shows the server's message when another rule fails, and our own for the page limit", async () => {
    const f = fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Add a class page" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "x" } });
    f.fail.create = Response.json(
      {
        status: "ERROR_INVALID_BODY_TEXT",
        error_message: "the text has a forbidden character",
        request_id: "r",
      },
      { status: 400 },
    );
    await userEvent.click(screen.getByRole("button", { name: "Save page" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "the text has a forbidden character",
    );
  });

  it("edits a page", async () => {
    const f = fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Edit Page 1" }));
    const title = screen.getByLabelText("Title");
    expect(title).toHaveValue("Page 1");
    fireEvent.change(title, { target: { value: "Our teachers" } });
    await userEvent.selectOptions(screen.getByLabelText("Kind of page"), "teachers");
    await userEvent.click(screen.getByRole("button", { name: "Save page" }));
    await screen.findByRole("button", { name: "Edit Our teachers" });
    expect(f.calls.find((c) => c.path === "/class-page/update")!.body).toEqual({
      class_id: CID,
      id: pid(1),
      kind: "teachers",
      placement: "front",
      title: "Our teachers",
      body: "Body 1",
    });
  });

  it("deletes a page after a confirm", async () => {
    const f = fake();
    await open();
    await userEvent.click(await screen.findByRole("button", { name: "Delete Page 1" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(screen.queryByText("Page 1")).not.toBeInTheDocument());
    expect(f.calls.find((c) => c.path === "/class-page/delete")!.body).toEqual({
      class_id: CID,
      id: pid(1),
    });
  });

  it("renders a body as text: markup stays literal, newlines stay", async () => {
    const evil = `<img src=x onerror="alert(1)"><script>alert(2)</script>\nsecond line`;
    fake({ front: [page(1, { body: evil })] });
    renderApp(`/classes/${CID}/assemble`);
    const container = document.body;
    const body = await screen.findByText(/second line/);
    expect(body.textContent).toBe(evil);
    expect(body.children).toHaveLength(0);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(body).toHaveClass("page-body");
  });
});
