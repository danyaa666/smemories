import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp } from "../test/api";

const fields = [
  {
    id: "name",
    kind: "short_text",
    label: { en: "Your name", vi: "Tên của bạn" },
    hint: { en: "e.g. Minh Khoa", vi: "VD: Minh Khoa" },
    required: true,
    max_length: 60,
  },
  {
    id: "relationship",
    kind: "short_text",
    label: { en: "Your relationship", vi: "Mối quan hệ" },
    hint: { en: "roommate", vi: "bạn cùng phòng" },
    required: false,
    max_length: 60,
  },
  {
    id: "message",
    kind: "long_text",
    label: { en: "Your message", vi: "Lời nhắn" },
    hint: { en: "A memory…", vi: "Một kỷ niệm…" },
    required: true,
    max_length: 2000,
  },
];
const formData = {
  yearbook: { title: "Linh's yearbook" },
  owner: { display_name: "Linh" },
  deadline_at: null,
  open: true,
  fields,
};
const GET = "GET /public-collect/get?token=abc";
const load = () => mockApi({ [GET]: () => ok(formData) });

// A fake XHR: each send() takes the next queued reply.
type Reply = (x: FakeXHR) => void;
class FakeXHR {
  static sent: FakeXHR[] = [];
  static replies: Reply[] = [];
  upload: { onprogress?: (e: unknown) => void } = {};
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  status = 0;
  responseText = "";
  url = "";
  form?: FormData;
  private headers: Record<string, string> = {};
  open(_m: string, url: string) {
    this.url = url;
  }
  setRequestHeader() {}
  getResponseHeader(n: string) {
    return this.headers[n.toLowerCase()] ?? null;
  }
  send(f: FormData) {
    this.form = f;
    FakeXHR.sent.push(this);
    const r = FakeXHR.replies.shift();
    queueMicrotask(() => r?.(this));
  }
  respond(status: number, body: unknown, headers: Record<string, string> = {}) {
    this.status = status;
    this.responseText = JSON.stringify(body);
    this.headers = headers;
    this.onload?.();
  }
}
const accept: Reply = (x) =>
  x.respond(200, { status: "OK", data: { id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY" } });
const reject =
  (status: number, code: string, message = code, headers?: Record<string, string>): Reply =>
  (x) =>
    x.respond(status, { status: code, error_message: message, request_id: "r" }, headers);
const networkError: Reply = (x) => x.onerror?.();
const reply = (...r: Reply[]) => FakeXHR.replies.push(...r);

const user = () => userEvent.setup({ applyAccept: false });
const photo = (name: string, type = "image/jpeg", size = 1000) => {
  const f = new File(["x"], name, { type });
  Object.defineProperty(f, "size", { value: size });
  return f;
};

beforeEach(() => {
  FakeXHR.sent = [];
  FakeXHR.replies = [];
  vi.stubGlobal("XMLHttpRequest", FakeXHR);
  let n = 0;
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: () => `blob:p${n++}`, revokeObjectURL: () => {} }),
  );
});
afterEach(() => vi.useRealTimers());

async function fill(u: ReturnType<typeof user>, name = "Khoa", message = "Thanks for everything") {
  const nameInput = await screen.findByLabelText(/Your name/);
  if (name) await u.type(nameInput, name);
  if (message) await u.type(screen.getByLabelText(/Your message/), message);
}
const send = (u: ReturnType<typeof user>) =>
  u.click(screen.getByRole("button", { name: "Send my note" }));

describe("public note form", () => {
  it("builds the form from the API fields in the page language, with optional marks and counters", async () => {
    load();
    renderApp("/n/abc");
    expect(await screen.findByRole("heading", { name: "Linh's yearbook" })).toBeInTheDocument();
    expect(screen.getByLabelText("Your name")).toHaveAttribute("placeholder", "e.g. Minh Khoa");
    expect(screen.getByLabelText("Your relationship (optional)")).toBeInTheDocument();
    expect(screen.getByLabelText("Your message")).toHaveAttribute("maxlength", "2000");
    expect(screen.getByText("0/2000")).toBeInTheDocument();
    expect(screen.getByText(/Only Linh will see your note/)).toBeInTheDocument();
    await act(() => i18n.changeLanguage("vi"));
    expect(screen.getByLabelText("Tên của bạn")).toHaveAttribute("placeholder", "VD: Minh Khoa");
    expect(screen.getByLabelText("Mối quan hệ (không bắt buộc)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Gửi lời nhắn" })).toBeInTheDocument();
  });

  it("does not send with a required field empty: names it, focuses the first, no request", async () => {
    load();
    renderApp("/n/abc");
    const u = user();
    await fill(u, "", "Hello");
    await send(u);
    expect(await screen.findByText("Write a few words before sending.")).toBeInTheDocument();
    expect(screen.getByLabelText("Your name")).toHaveFocus();
    expect(FakeXHR.sent).toHaveLength(0);
  });

  it("sends answers before photos, trims, skips empty optional fields, then shows the sent screen", async () => {
    load();
    reply(accept);
    renderApp("/n/abc");
    const u = user();
    await fill(u, "  Khoa ", "Hi\nthere");
    await u.upload(document.querySelector<HTMLInputElement>('input[type="file"]')!, [
      photo("a.jpg"),
      photo("b.png", "image/png"),
    ]);
    expect(screen.getByRole("img", { name: "Your photo 2" })).toBeInTheDocument();
    await send(u);
    expect(
      await screen.findByRole("heading", { name: "Sent! Thank you, Khoa." }),
    ).toBeInTheDocument();
    const x = FakeXHR.sent[0]!;
    expect(x.url).toBe("/api/public-collect/submit-note?token=abc");
    expect([...x.form!.keys()]).toEqual(["answers", "photos", "photos"]);
    expect(JSON.parse(x.form!.get("answers") as string)).toEqual({
      name: "Khoa",
      message: "Hi\nthere",
    });
    expect(screen.getByRole("link", { name: "Make your own yearbook" })).toHaveAttribute(
      "href",
      "/register",
    );
    expect(window.localStorage.getItem("smemories.note-draft.abc")).toBeNull();
    await u.click(screen.getByRole("button", { name: "Write another note" }));
    expect(screen.getByLabelText("Your name")).toHaveValue("");
  });

  it("sends the honeypot part only when something filled it", async () => {
    load();
    reply(accept);
    renderApp("/n/abc");
    const u = user();
    await fill(u);
    await u.type(document.querySelector<HTMLInputElement>('input[name="website"]')!, "http://spam");
    await send(u);
    await screen.findByText(/Sent! Thank you/);
    expect([...FakeXHR.sent[0]!.form!.keys()]).toEqual(["answers", "website"]);
  });

  it("keeps the text answers in localStorage while open and restores them", async () => {
    load();
    const first = renderApp("/n/abc");
    const u = user();
    await fill(u, "Khoa", "draft text");
    expect(JSON.parse(window.localStorage.getItem("smemories.note-draft.abc")!)).toMatchObject({
      message: "draft text",
    });
    first.unmount();
    renderApp("/n/abc");
    expect(await screen.findByLabelText("Your message")).toHaveValue("draft text");
  });

  it("works when storage is blocked", async () => {
    load();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    reply(accept);
    renderApp("/n/abc");
    const u = user();
    await fill(u);
    await send(u);
    expect(await screen.findByText(/Sent! Thank you/)).toBeInTheDocument();
  });

  describe("photos", () => {
    const input = () => document.querySelector<HTMLInputElement>('input[type="file"]')!;

    it("checks type, size and count in the browser", async () => {
      load();
      renderApp("/n/abc");
      const u = user();
      await screen.findByLabelText("Your name");
      await u.upload(input(), photo("pic.heic", "image/heic"));
      expect(screen.getByRole("alert")).toHaveTextContent(
        "pic.heic: only JPG, PNG or WebP photos work.",
      );
      await u.upload(input(), photo("big.jpg", "image/jpeg", 10 * 1024 * 1024 + 1));
      expect(screen.getByRole("alert")).toHaveTextContent("big.jpg is larger than 10 MB.");
      expect(screen.queryByRole("img")).not.toBeInTheDocument();
      await u.upload(input(), [photo("1.jpg"), photo("2.jpg"), photo("3.jpg"), photo("4.jpg")]);
      expect(screen.getAllByRole("img")).toHaveLength(3);
      expect(screen.getByRole("alert")).toHaveTextContent("You can add up to 3 photos.");
      expect(screen.queryByText("Add photo")).not.toBeInTheDocument();
      await u.click(screen.getByRole("button", { name: "Remove photo 1" }));
      expect(screen.getAllByRole("img")).toHaveLength(2);
    });

    it("shows the photo panel on a rejection without naming a photo; Send without photos resends the text only", async () => {
      load();
      reply(reject(400, "ERROR_INVALID_IMAGE"), accept);
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await u.upload(input(), [photo("a.jpg"), photo("b.jpg")]);
      await send(u);
      const panel = await screen.findByText("Not sent");
      expect(panel.parentElement).toHaveTextContent("A photo didn't upload. Your words are fine.");
      expect(screen.getByLabelText("Your name")).toHaveValue("Khoa");
      await u.click(screen.getByRole("button", { name: "Send without photos" }));
      await screen.findByText(/Sent! Thank you/);
      expect([...FakeXHR.sent[1]!.form!.keys()]).toEqual(["answers"]);
    });

    it("Try again resends with the photos; Remove photos clears them", async () => {
      load();
      reply(reject(415, "ERROR_UNSUPPORTED_MEDIA_TYPE"), reject(400, "ERROR_INVALID_IMAGE"));
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await u.upload(input(), photo("a.jpg"));
      await send(u);
      await u.click(await screen.findByRole("button", { name: "Try again" }));
      await waitFor(() => expect(FakeXHR.sent).toHaveLength(2));
      expect([...FakeXHR.sent[1]!.form!.keys()]).toEqual(["answers", "photos"]);
      await u.click(await screen.findByRole("button", { name: "Remove photos" }));
      expect(screen.queryByRole("img")).not.toBeInTheDocument();
      expect(screen.queryByText("Not sent")).not.toBeInTheDocument();
    });
  });

  it("while sending: Send is disabled, a progress bar shows, and a slow connection gets a calm message", async () => {
    load();
    reply(() => {}); // the server never answers
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderApp("/n/abc");
    const u = userEvent.setup({ applyAccept: false, advanceTimers: vi.advanceTimersByTime });
    await fill(u);
    await send(u);
    expect(screen.getByRole("button", { name: "Sending…" })).toBeDisabled();
    act(() =>
      FakeXHR.sent[0]!.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 4 }),
    );
    expect(screen.getByRole("progressbar", { name: "Sending your note" })).toHaveValue(0.25);
    expect(screen.queryByText(/Slow connection/)).not.toBeInTheDocument();
    act(() => void vi.advanceTimersByTime(6100));
    expect(screen.getByText(/Slow connection. Still sending/)).toHaveTextContent(
      "saved on this phone",
    );
  });

  describe("failures", () => {
    it("503 busy is resent after Retry-After and then succeeds", async () => {
      load();
      reply(reject(503, "ERROR_BUSY", "busy", { "Retry-After": "1" }), accept);
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await send(u);
      expect(await screen.findByText(/Still sending. The server is busy/)).toBeInTheDocument();
      await screen.findByText(/Sent! Thank you/, undefined, { timeout: 3000 });
      expect(FakeXHR.sent).toHaveLength(2);
    });

    it("a network error keeps the answers, sends once, and the user presses Send again", async () => {
      load();
      reply(networkError, accept);
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await send(u);
      expect(await screen.findByRole("alert")).toHaveTextContent("Could not reach the server");
      expect(FakeXHR.sent).toHaveLength(1);
      expect(screen.getByLabelText("Your name")).toHaveValue("Khoa");
      expect(screen.getByRole("button", { name: "Send my note" })).toBeEnabled();
      await send(u);
      await screen.findByText(/Sent! Thank you/);
      expect(FakeXHR.sent).toHaveLength(2);
    });

    it("408 request timeout asks to press Send again and keeps the form", async () => {
      load();
      reply(reject(408, "ERROR_REQUEST_TIMEOUT"));
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await send(u);
      expect(await screen.findByRole("alert")).toHaveTextContent("The connection stalled");
      expect(screen.getByLabelText("Your message")).toHaveValue("Thanks for everything");
    });

    it("shows missing_answer and invalid_answer next to the field the server names", async () => {
      load();
      reply(
        reject(400, "ERROR_MISSING_ANSWER", "missing answer for field name"),
        reject(400, "ERROR_INVALID_ANSWER", "invalid answer for field message"),
      );
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await send(u);
      const nameField = (await screen.findByLabelText("Your name")).closest(
        ".field",
      ) as HTMLElement;
      expect(
        await within(nameField).findByText("Write a few words before sending."),
      ).toBeInTheDocument();
      await send(u);
      const msgField = screen.getByLabelText("Your message").closest(".field") as HTMLElement;
      expect(await within(msgField).findByText(/characters we can't accept/)).toBeInTheDocument();
    });

    it("a full link on submit shows the full screen with the owner's name and no number", async () => {
      load();
      reply(reject(409, "ERROR_COLLECTION_FULL"));
      renderApp("/n/abc");
      const u = user();
      await fill(u);
      await send(u);
      expect(await screen.findByRole("heading", { name: "This link is full" })).toBeInTheDocument();
      expect(
        screen.getByText("This link has reached its limit. Ask Linh to open a new link."),
      ).toBeInTheDocument();
    });
  });

  describe("dead links", () => {
    it("404 (revoked, unknown, malformed) shows one screen with no name and no form", async () => {
      mockApi({ [GET]: () => err(404, "not_found") });
      renderApp("/n/abc");
      expect(
        await screen.findByRole("heading", { name: "This link doesn't work anymore" }),
      ).toBeInTheDocument();
      expect(screen.queryByLabelText(/Your name/)).not.toBeInTheDocument();
      expect(screen.queryByText(/Linh/)).not.toBeInTheDocument();
    });

    it("410 expired is the closed screen, before the friend writes anything", async () => {
      mockApi({ [GET]: () => err(410, "collection_closed") });
      renderApp("/n/abc");
      expect(
        await screen.findByRole("heading", { name: "This link has closed" }),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Send my note" })).not.toBeInTheDocument();
    });

    it("409 full on the lookup is the full screen", async () => {
      mockApi({ [GET]: () => err(409, "collection_full") });
      renderApp("/n/abc");
      expect(await screen.findByRole("heading", { name: "This link is full" })).toBeInTheDocument();
      expect(screen.getByText(/Ask the owner to open a new link/)).toBeInTheDocument();
    });

    it("any other lookup failure offers Try again", async () => {
      let n = 0;
      mockApi({ [GET]: () => (n++ ? ok(formData) : err(500, "internal_error")) });
      renderApp("/n/abc");
      const u = user();
      await u.click(await screen.findByRole("button", { name: "Try again" }));
      expect(await screen.findByLabelText("Your name")).toBeInTheDocument();
    });

    it("needs no session: the page never asks for the current user", async () => {
      const calls = load();
      renderApp("/n/abc");
      await screen.findByLabelText("Your name");
      expect(calls.map((c) => c.path)).toEqual(["/public-collect/get?token=abc"]);
    });
  });
});
