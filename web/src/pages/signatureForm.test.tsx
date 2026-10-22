import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NoteItem } from "../api/notes";
import { NoteCard } from "../components/NoteCard";
import i18n from "../i18n";
import { mockApi, ok, renderApp } from "../test/api";

// The pad on the public form (T-096) and the signature on the inbox card, with a fake canvas and a fake XHR.
const fields = [
  {
    id: "name",
    kind: "short_text",
    label: { en: "Your name", vi: "Tên" },
    hint: { en: "", vi: "" },
    required: true,
    max_length: 60,
  },
  {
    id: "message",
    kind: "long_text",
    label: { en: "Your message", vi: "Lời nhắn" },
    hint: { en: "", vi: "" },
    required: true,
    max_length: 2000,
  },
];
const info = { enabled: true, w: 1000, h: 400, max_strokes: 12, max_points: 600 };
const formData = (signature?: object) => ({
  yearbook: { title: "Linh's yearbook" },
  owner: { display_name: "Linh" },
  deadline_at: null,
  open: true,
  fields,
  ...(signature ? { signature } : {}),
});
const load = (signature?: object) =>
  mockApi({ "GET /public-collect/get?token=abc": () => ok(formData(signature)) });

class FakeXHR {
  static sent: FakeXHR[] = [];
  static status = 200;
  static body: unknown = { status: "OK", data: { id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY" } };
  upload = {};
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  status = 0;
  responseText = "";
  form?: FormData;
  open() {}
  setRequestHeader() {}
  getResponseHeader() {
    return null;
  }
  send(f: FormData) {
    this.form = f;
    FakeXHR.sent.push(this);
    queueMicrotask(() => {
      this.status = FakeXHR.status;
      this.responseText = JSON.stringify(FakeXHR.body);
      this.onload?.();
    });
  }
}

beforeEach(() => {
  FakeXHR.sent = [];
  FakeXHR.status = 200;
  FakeXHR.body = { status: "OK", data: { id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY" } };
  vi.stubGlobal("XMLHttpRequest", FakeXHR);
  // jsdom has no canvas: a context that records nothing, and a 500 x 200 box so a client pixel is half a unit
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
    new Proxy({}, { get: () => () => {}, set: () => true }) as unknown as CanvasRenderingContext2D,
  );
  vi.spyOn(HTMLCanvasElement.prototype, "getBoundingClientRect").mockReturnValue({
    left: 0,
    top: 0,
    width: 500,
    height: 200,
    right: 500,
    bottom: 200,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  });
});
afterEach(() => vi.useRealTimers());

const canvas = () => document.querySelector<HTMLCanvasElement>("canvas.sig-canvas")!;
const ptr = (id = 1) => ({ pointerId: id, isPrimary: true, button: 0 });
/** A stroke through these client points (a client pixel is two units). */
function stroke(points: [number, number][], id = 1) {
  const c = canvas();
  const [first, ...rest] = points;
  fireEvent.pointerDown(c, { ...ptr(id), clientX: first![0], clientY: first![1] });
  for (const [x, y] of rest) fireEvent.pointerMove(c, { ...ptr(id), clientX: x, clientY: y });
  const last = points[points.length - 1]!;
  fireEvent.pointerUp(c, { ...ptr(id), clientX: last[0], clientY: last[1] });
}
const along = (a: [number, number], b: [number, number], n = 10): [number, number][] =>
  Array.from({ length: n + 1 }, (_, i) => [
    a[0] + ((b[0] - a[0]) * i) / n,
    a[1] + ((b[1] - a[1]) * i) / n,
  ]);
const V: [number, number][] = [
  ...along([50, 20], [250, 100]),
  ...along([250, 100], [450, 20]).slice(1),
];

async function fill(u: ReturnType<typeof userEvent.setup>) {
  await u.type(await screen.findByLabelText(/Your name/), "Khoa");
  await u.type(screen.getByLabelText(/Your message/), "Thanks");
}
const sendBtn = () => screen.getByRole("button", { name: "Send my note" });

describe("signature pad on the public form", () => {
  it("is absent when the template does not enable it, and when the API says nothing", async () => {
    load({ ...info, enabled: false });
    const { unmount } = renderApp("/c/abc");
    await screen.findByLabelText(/Your name/);
    expect(screen.queryByText(/Sign here/)).toBeNull();
    expect(canvas()).toBeNull();
    unmount();
    load();
    renderApp("/c/abc");
    await screen.findByLabelText(/Your name/);
    expect(canvas()).toBeNull();
  });

  it("labels the pad as optional, keeps the canvas out of the tab order and out of the accessibility tree", async () => {
    load(info);
    renderApp("/c/abc");
    const group = await screen.findByRole("group", { name: "Sign here (optional)" });
    expect(canvas()).toHaveAttribute("aria-hidden", "true");
    expect(canvas()).not.toHaveAttribute("tabindex");
    expect(within(group).getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(within(group).getByRole("button", { name: "Clear" })).toBeDisabled();
    await act(() => i18n.changeLanguage("vi"));
    expect(
      screen.getByRole("group", { name: "Ký tên tại đây (không bắt buộc)" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hoàn tác" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Xóa" })).toBeInTheDocument();
  });

  it("sends nothing when the friend did not sign", async () => {
    load(info);
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    await u.click(sendBtn());
    await screen.findByRole("heading", { name: /Sent!/ });
    expect([...FakeXHR.sent[0]!.form!.keys()]).toEqual(["answers"]);
    expect(screen.queryByRole("img", { name: "Your signature" })).toBeNull();
  });

  it("draws, sends the canonical JSON as the multipart field and shows a receipt", async () => {
    load(info);
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    stroke(V);
    expect(screen.getByRole("status")).toHaveTextContent("Signature added");
    await u.click(sendBtn());
    const receipt = await screen.findByRole("img", { name: "Your signature" });
    const x = FakeXHR.sent[0]!;
    expect([...x.form!.keys()]).toEqual(["answers", "signature"]);
    expect(x.form!.get("signature")).toBe('{"v":1,"w":1000,"h":400,"s":[[100,40,500,200,900,40]]}');
    expect(receipt.querySelector("path")).toHaveAttribute("d", "M100 40L500 200L900 40");
    await u.click(screen.getByRole("button", { name: "Write another note" }));
    expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled(); // a fresh pad
  });

  it("maps the drawing space to the canvas's content box, inside its border (T-096 QA)", async () => {
    load(info);
    renderApp("/c/abc");
    await screen.findByLabelText(/Your name/);
    for (const k of ["clientLeft", "clientTop"]) Object.defineProperty(canvas(), k, { value: 2 }); // a 2 px border: the content box is 496 x 196 at (2, 2)
    stroke([
      [2, 2],
      [250, 100],
      [498, 2],
    ]);
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    await u.click(sendBtn());
    expect(FakeXHR.sent[0]!.form!.get("signature")).toBe(
      '{"v":1,"w":1000,"h":400,"s":[[0,0,500,200,1000,0]]}',
    );
  });

  it("undoes the last stroke, then clears, and announces it", async () => {
    load(info);
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    stroke(V);
    stroke(along([50, 150], [450, 150]));
    await u.click(screen.getByRole("button", { name: "Undo" }));
    await u.click(sendBtn());
    await screen.findByRole("img", { name: "Your signature" });
    expect(FakeXHR.sent[0]!.form!.get("signature")).toBe(
      '{"v":1,"w":1000,"h":400,"s":[[100,40,500,200,900,40]]}',
    );
  });

  it("Clear empties the pad: nothing is sent", async () => {
    load(info);
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    stroke(V);
    await u.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.getByRole("status")).toHaveTextContent("Signature cleared");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    await u.click(sendBtn());
    await screen.findByRole("heading", { name: /Sent!/ });
    expect(FakeXHR.sent[0]!.form!.has("signature")).toBe(false);
  });

  it("refuses a stroke over the limits with the localised message and keeps what was drawn", async () => {
    load({ ...info, max_strokes: 2 });
    renderApp("/c/abc");
    await screen.findByRole("group", { name: /Sign here/ });
    stroke(V);
    stroke(along([50, 150], [450, 150]));
    expect(screen.queryByRole("alert")).toBeNull();
    stroke(along([50, 180], [450, 180])); // the third
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Your signature is too detailed. Clear it and sign more simply.",
    );
    await act(() => i18n.changeLanguage("vi"));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Chữ ký quá chi tiết. Hãy xóa và ký đơn giản hơn.",
    );
    expect(screen.getByRole("button", { name: "Hoàn tác" })).toBeEnabled();
  });

  it("refuses a stroke that does not fit the point cap even when thinned", async () => {
    load({ ...info, max_points: 4 });
    renderApp("/c/abc");
    await screen.findByRole("group", { name: /Sign here/ });
    // a zigzag of 20-unit teeth: far more than 4 points whatever the epsilon
    stroke(
      Array.from({ length: 20 }, (_, i) => [20 + i * 10, i % 2 ? 40 : 60] as [number, number]),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled(); // nothing was added
  });

  it("ignores a second pointer while one stroke is down, and a secondary mouse button", async () => {
    load(info);
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    const c = canvas();
    fireEvent.pointerDown(c, {
      pointerId: 2,
      isPrimary: true,
      button: 2,
      clientX: 10,
      clientY: 10,
    });
    fireEvent.pointerUp(c, { pointerId: 2, isPrimary: true, button: 2, clientX: 200, clientY: 10 });
    expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
    fireEvent.pointerDown(c, { ...ptr(1), clientX: 50, clientY: 20 });
    fireEvent.pointerDown(c, {
      pointerId: 3,
      isPrimary: false,
      button: 0,
      clientX: 300,
      clientY: 150,
    });
    for (const [x, y] of V.slice(1))
      fireEvent.pointerMove(c, { ...ptr(1), clientX: x, clientY: y });
    fireEvent.pointerUp(c, { ...ptr(1), clientX: 450, clientY: 20 });
    await u.click(sendBtn());
    await screen.findByRole("heading", { name: /Sent!/ });
    expect(FakeXHR.sent[0]!.form!.get("signature")).toBe(
      '{"v":1,"w":1000,"h":400,"s":[[100,40,500,200,900,40]]}',
    );
  });

  it("drops a cancelled stroke", async () => {
    load(info);
    renderApp("/c/abc");
    await screen.findByRole("group", { name: /Sign here/ });
    const c = canvas();
    fireEvent.pointerDown(c, { ...ptr(), clientX: 50, clientY: 20 });
    fireEvent.pointerMove(c, { ...ptr(), clientX: 250, clientY: 100 });
    fireEvent.pointerCancel(c, ptr());
    fireEvent.pointerUp(c, { ...ptr(), clientX: 250, clientY: 100 });
    expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
  });

  it("shows ERROR_INVALID_SIGNATURE in the page language and keeps the pad so the friend can retry", async () => {
    load(info);
    FakeXHR.status = 400;
    FakeXHR.body = {
      status: "ERROR_INVALID_SIGNATURE",
      error_message: "signature: the signature is too small",
      request_id: "r",
    };
    renderApp("/c/abc");
    const u = userEvent.setup({ applyAccept: false });
    await fill(u);
    stroke(V);
    await u.click(sendBtn());
    expect(
      await screen.findByText("We couldn't use this signature. Clear it and sign again."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/too small/)).toBeNull(); // the server's rule text is not shown
    expect(screen.getByRole("alert", { name: "" })).toHaveClass("sig-error"); // spaced from the Photos heading below
    expect(screen.getByRole("button", { name: "Clear" })).toBeEnabled();
    await u.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.queryByText(/couldn't use this signature/)).toBeNull();
  });
});

const note = (signature: unknown): NoteItem =>
  ({
    id: "01J9Z3K6V8Q4M7N2P5R8T0N001",
    status: "pending",
    answers: { name: "Linh", message: "Hi" },
    signature,
    photos: [],
    link_label: "",
    created_at: Date.now() - 3600_000,
    position: null,
  }) as NoteItem;
const card = (n: NoteItem) =>
  render(
    <ul>
      <NoteCard
        note={n}
        lang="en"
        selected={false}
        onSelect={() => {}}
        onStatus={() => {}}
        onDelete={() => {}}
      />
    </ul>,
  );

describe("signature on the inbox card", () => {
  it("draws the stored integers as an inline SVG path with an accessible name", () => {
    card(note({ v: 1, w: 1000, h: 400, s: [[100, 40, 500, 200, 900, 40]] }));
    const svg = screen.getByRole("img", { name: "Signature of Linh" });
    expect(svg.tagName.toLowerCase()).toBe("svg");
    expect(svg.querySelector("path")).toHaveAttribute("d", "M100 40L500 200L900 40");
    expect(svg.querySelectorAll("*")).toHaveLength(1); // one path, no other element
    expect(document.querySelector("img[src^='data:']")).toBeNull();
  });
  it("renders nothing for no signature or for anything that is not valid integers in range", () => {
    for (const bad of [
      null,
      undefined,
      { v: 1, w: 1000, h: 400, s: [[0, 0, 50.5, 0]] },
      { v: 1, w: 1000, h: 400, s: [[0, 0, 5000, 0]] },
      { v: 1, w: 1000, h: 400, s: [[0, 0, '1"/><script>alert(1)</script>', 0]] },
      "M0 0L9 9",
    ]) {
      const { unmount } = card(note(bad));
      expect(screen.queryByRole("img", { name: /Signature of/ })).toBeNull();
      expect(document.querySelector("script")).toBeNull();
      unmount();
    }
  });
  it("names the signature in Vietnamese", async () => {
    await act(() => i18n.changeLanguage("vi"));
    card(note({ v: 1, w: 1000, h: 400, s: [[100, 40, 500, 200]] }));
    expect(screen.getByRole("img", { name: "Chữ ký của Linh" })).toBeInTheDocument();
  });
});
