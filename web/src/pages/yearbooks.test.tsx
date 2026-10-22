import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Yearbook } from "../api/yearbooks";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

const ID = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const PHOTO = "01J9Z3K6V8Q4M7N2P5R8T0PPPP";
const OTHER = "01J9Z3K6V8Q4M7N2P5R8T0QQQQ";

const yb = (
  over: Partial<Yearbook> = {},
  profile: Partial<Yearbook["profile"]> = {},
): Yearbook => ({
  id: ID,
  title: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  class_name: "12A1",
  graduation_year: 2026,
  motto: "",
  language: "vi",
  page_size: "A4",
  template_id: null,
  cover_media_id: null,
  created_at: 1760000000000,
  updated_at: 1760086400000,
  profile: {
    id: "01J9Z3K6V8Q4M7N2P5R8T0BBBB",
    is_owner: true,
    full_name: "Lan Nguyễn",
    nickname: "Lan",
    birthday: "2008-03-04",
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
    ...profile,
  },
  ...over,
});
const me = () => ok(user());
const book = (y: Yearbook) => ok(y);
const page = (items: Yearbook[], next = "") => ok({ items, next_id: next });
const fail = (status: number, code: string) => Response.json(apiErr(code), { status });
const where = () => screen.getByTestId("where").textContent;
const last = <T,>(a: T[]) => a[a.length - 1]!;

describe("yearbook list", () => {
  it("lists the books with a cover thumbnail, and pages with next_id", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      "GET /yearbook/get-list": page([yb({ cover_media_id: PHOTO })], "c2"),
      "GET /yearbook/get-list?next_id=c2": page([yb({ id: OTHER, title: "Second" })]),
    });
    const { container } = renderApp("/yearbooks");
    expect(await screen.findByRole("link", { name: "Lớp 12A1" })).toHaveAttribute(
      "href",
      `/yearbooks/${ID}`,
    );
    expect(screen.getByText("THPT Chu Văn An · 12A1 · 2026")).toBeInTheDocument();
    expect(container.querySelector("img")).toHaveAttribute(
      "src",
      `/api/media/get-content?id=${PHOTO}&variant=thumb`,
    );
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByRole("link", { name: "Second" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(calls.at(-1)?.path).toBe("/yearbook/get-list?next_id=c2");
  });

  it("invites the user to create a first book", async () => {
    mockApi({
      "GET /user/get-me": me,
      "GET /yearbook/get-list": page([]),
    });
    renderApp("/yearbooks");
    expect(await screen.findByText(/no yearbooks yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New yearbook" })).toHaveAttribute(
      "href",
      "/yearbooks/new",
    );
  });

  it("sends a signed-out visitor to sign in", async () => {
    mockApi({ "GET /user/get-me": err(401, "unauthenticated") });
    renderApp("/yearbooks");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
  });
});

describe("create", () => {
  it("offers A5, A4 and Letter, posts the form and opens the new book", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      "POST /yearbook/create": () => book(yb({ page_size: "Letter" })),
      [`GET /yearbook/get?id=${ID}`]: book(yb({ page_size: "Letter" })),
      "GET /yearbook/get-list": page([]),
    });
    renderApp("/yearbooks/new");
    await userEvent.type(await screen.findByLabelText("Title"), "Our year 🎓");
    await userEvent.type(screen.getByLabelText("Graduation year"), "2026");
    const size = screen.getByLabelText("Page size");
    expect(
      within(size)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["A5 (148 × 210 mm)", "A4 (210 × 297 mm)", "US Letter (8.5 × 11 in)"]);
    await userEvent.selectOptions(size, "Letter");
    await userEvent.click(screen.getByRole("button", { name: "Create yearbook" }));
    expect(await screen.findByRole("heading", { name: "Lớp 12A1" })).toBeInTheDocument();
    expect(where()).toBe(`/yearbooks/${ID}`);
    expect(calls.find((c) => c.path === "/yearbook/create")?.body).toEqual({
      title: "Our year 🎓",
      school_name: "",
      class_name: "",
      graduation_year: 2026,
      motto: "",
      language: "en",
      page_size: "Letter",
    });
  });

  it("defaults the book language to the interface language", async () => {
    await i18n.changeLanguage("vi");
    mockApi({ "GET /user/get-me": me });
    renderApp("/yearbooks/new");
    expect(await screen.findByLabelText("Ngôn ngữ của kỷ yếu")).toHaveValue("vi");
  });

  it("shows a field error next to the field and a limit error above the button", async () => {
    let n = 0;
    mockApi({
      "GET /user/get-me": me,
      "POST /yearbook/create": () =>
        n++ === 0 ? fail(400, "invalid_title") : fail(409, "limit_reached"),
    });
    renderApp("/yearbooks/new");
    await userEvent.click(await screen.findByRole("button", { name: "Create yearbook" }));
    const title = await screen.findByLabelText("Title");
    await waitFor(() => expect(title).toHaveAttribute("aria-invalid", "true"));
    expect(screen.getByRole("alert")).toHaveTextContent("title of 1 to 120");
    await userEvent.click(screen.getByRole("button", { name: "Create yearbook" }));
    expect(await screen.findByText(/at most 20 yearbooks/)).toBeInTheDocument();
    expect(title).not.toHaveAttribute("aria-invalid");
  });
});

describe("edit", () => {
  it("saving the profile keeps the profile photo (update-profile replaces everything)", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      [`GET /yearbook/get?id=${ID}`]: book(yb({}, { photo_media_id: PHOTO })),
      ["POST /yearbook/update-profile"]: (c) =>
        book(yb({}, { ...(c.body as object), photo_media_id: PHOTO })),
      "GET /yearbook/get-list": page([]),
      [MEDIA]: mediaPage([PHOTO]),
    });
    renderApp(`/yearbooks/${ID}`);
    const name = await screen.findByLabelText("Full name");
    await userEvent.clear(name);
    await userEvent.type(name, "Nguyễn Thị Lan");
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    expect(last(calls.filter((c) => c.path === "/yearbook/update-profile")).body).toEqual({
      yearbook_id: ID,
      full_name: "Nguyễn Thị Lan",
      nickname: "Lan",
      birthday: "2008-03-04",
      quote: "",
      hobbies: "",
      future_plans: "",
      photo_media_id: PHOTO,
    });
    expect(
      screen.getByRole("button", { name: "Profile photo", pressed: true }),
    ).toBeInTheDocument();
  });

  it("saving the book details sends no cover (an absent cover is kept) and clears the year with null", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      [`GET /yearbook/get?id=${ID}`]: book(yb({ cover_media_id: PHOTO })),
      ["POST /yearbook/update"]: (c) => book(yb({ ...(c.body as object), cover_media_id: PHOTO })),
      "GET /yearbook/get-list": page([]),
      [MEDIA]: mediaPage([PHOTO]),
    });
    renderApp(`/yearbooks/${ID}`);
    await userEvent.clear(await screen.findByLabelText("Graduation year"));
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    const body = last(calls.filter((c) => c.path === "/yearbook/update")).body as Record<
      string,
      unknown
    >;
    expect(body.id).toBe(ID);
    expect(body.graduation_year).toBeNull();
    expect(body.page_size).toBe("A4");
    expect(body).not.toHaveProperty("cover_media_id");
    expect(screen.getByRole("button", { name: "Cover photo", pressed: true })).toBeInTheDocument();
  });

  it("says so when the book does not exist", async () => {
    mockApi({ "GET /user/get-me": me, [`GET /yearbook/get?id=${ID}`]: fail(404, "not_found") });
    renderApp(`/yearbooks/${ID}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("does not exist");
  });

  it("deletes the book after confirmation and returns to the list", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const calls = mockApi({
      "GET /user/get-me": me,
      [`GET /yearbook/get?id=${ID}`]: book(yb()),
      "POST /yearbook/delete": ok({}),
      "GET /yearbook/get-list": page([]),
    });
    renderApp(`/yearbooks/${ID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Delete yearbook" }));
    expect(await screen.findByRole("heading", { name: "My yearbooks" })).toBeInTheDocument();
    expect(where()).toBe("/yearbooks");
    expect(calls.find((c) => c.path === "/yearbook/delete")?.body).toEqual({ id: ID });
  });

  it("does not delete when the user cancels", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const calls = mockApi({ "GET /user/get-me": me, [`GET /yearbook/get?id=${ID}`]: book(yb()) });
    renderApp(`/yearbooks/${ID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Delete yearbook" }));
    expect(calls.some((c) => c.path === "/yearbook/delete")).toBe(false);
  });
});

// A hand-driven XMLHttpRequest: the test decides when progress and the answer arrive.
class FakeXhr {
  static all: FakeXhr[] = [];
  upload: {
    onprogress?: (e: { lengthComputable: boolean; loaded: number; total: number }) => void;
  } = {};
  onload?: () => void;
  onerror?: () => void;
  status = 0;
  responseText = "";
  url = "";
  form?: FormData;
  private headers: Record<string, string> = {};
  constructor() {
    FakeXhr.all.push(this);
  }
  open(method: string, url: string) {
    expect(method).toBe("POST");
    this.url = url;
  }
  setRequestHeader() {}
  getResponseHeader(n: string) {
    return this.headers[n] ?? null;
  }
  send(form: FormData) {
    this.form = form;
  }
  progress(f: number) {
    act(() => this.upload.onprogress?.({ lengthComputable: true, loaded: f * 100, total: 100 }));
  }
  reply(status: number, body: unknown, headers: Record<string, string> = {}) {
    this.status = status;
    this.headers = headers;
    this.responseText = JSON.stringify(body);
    act(() => this.onload?.());
  }
}
// The v2 error body: legacy snake_case test names map to ERROR_ + upper case, except the merged codes.
const v2 = (code: string) =>
  code === "payload_too_large" ? "ERROR_TOO_LARGE" : `ERROR_${code.toUpperCase()}`;
const apiErr = (code: string) => ({ status: v2(code), error_message: code, request_id: "r1" });
const png = (name = "a.png") => new File([new Uint8Array([1, 2, 3])], name, { type: "image/png" });
const item = (id: string) => ({
  id,
  yearbook_id: ID,
  content_type: "image/jpeg",
  bytes: 3,
  width: 800,
  height: 600,
  created_at: 1760000000000,
});
const created = { status: "OK", data: item(PHOTO) };
const MEDIA = `GET /media/get-list?yearbook_id=${ID}`;
const mediaPage = (ids: string[], next = "") =>
  Response.json({ status: "OK", data: { items: ids.map(item), next_id: next } });

async function openEditor(extra: Record<string, Response | ((c: Call) => Response)> = {}) {
  FakeXhr.all = [];
  vi.stubGlobal("XMLHttpRequest", FakeXhr);
  const calls = mockApi({
    "GET /user/get-me": me,
    [`GET /yearbook/get?id=${ID}`]: book(yb()),
    "GET /yearbook/get-list": page([]),
    [MEDIA]: mediaPage([]),
    ...extra,
  });
  const { container } = renderApp(`/yearbooks/${ID}`);
  await screen.findByRole("heading", { name: "Photos" });
  const input = container.querySelector<HTMLInputElement>('input[type="file"]')!;
  return { calls, input };
}

describe("photos", () => {
  it("uploads the file in the multipart field `file`, shows progress, then a thumbnail", async () => {
    let uploaded = false;
    const { input } = await openEditor({ [MEDIA]: () => mediaPage(uploaded ? [PHOTO] : []) });
    await userEvent.upload(input, png("holiday.png"));
    await waitFor(() => expect(FakeXhr.all).toHaveLength(1));
    const x = FakeXhr.all[0]!;
    expect(x.url).toBe("/api/media/create");
    expect([...x.form!.keys()]).toEqual(["yearbook_id", "file"]); // the target comes first
    expect(x.form!.get("yearbook_id")).toBe(ID);
    expect((x.form!.get("file") as File).name).toBe("holiday.png");
    x.progress(0.5);
    expect(screen.getByRole("progressbar", { name: "holiday.png" })).toHaveValue(0.5);
    uploaded = true;
    x.reply(200, created);
    const img = await screen.findByRole("img", { name: "Photo 1" });
    expect(img).toHaveAttribute("src", `/api/media/get-content?id=${PHOTO}&variant=thumb`);
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it.each([
    [413, "payload_too_large", {}, "too large", "quá lớn"],
    [415, "unsupported_media_type", {}, "JPEG, PNG and WebP", "JPEG, PNG và WebP"],
    [400, "invalid_image", {}, "could not be read", "Không đọc được"],
    [409, "quota_exceeded", {}, "photo limit", "giới hạn ảnh"],
    [429, "rate_limited", { "Retry-After": "120" }, "Try again in 2 min", "thử lại sau 2 phút"],
    [403, "email_not_verified", {}, "Verify your email address first", "xác minh địa chỉ email"], // T-110: photo uploads need a verified email
  ])("explains %i %s in the user's language", async (status, code, headers, en, vi) => {
    const { input } = await openEditor();
    await userEvent.upload(input, png());
    await waitFor(() => expect(FakeXhr.all).toHaveLength(1));
    FakeXhr.all[0]!.reply(status, apiErr(code), headers);
    expect(await screen.findByText(new RegExp(en))).toBeInTheDocument();
    await act(() => i18n.changeLanguage("vi")); // the message follows the language switch
    expect(await screen.findByText(new RegExp(vi))).toBeInTheDocument();
  });

  it("reads a connection reset on an oversized file as payload_too_large", async () => {
    const { input } = await openEditor();
    const big = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "big.png", { type: "image/png" });
    await userEvent.upload(input, big);
    await waitFor(() => expect(FakeXhr.all).toHaveLength(1));
    act(() => FakeXhr.all[0]!.onerror?.());
    expect(await screen.findByText(/too large/)).toBeInTheDocument();
  });

  it("retries 503 busy after the wait and then succeeds", async () => {
    const delays: number[] = [];
    const real = globalThis.setTimeout;
    vi.spyOn(globalThis, "setTimeout").mockImplementation(((fn: () => void, ms?: number) => {
      if (ms === 2000) delays.push(ms);
      return real(fn, ms === 2000 ? 0 : ms);
    }) as typeof setTimeout);
    let uploaded = false;
    const { input } = await openEditor({ [MEDIA]: () => mediaPage(uploaded ? [PHOTO] : []) });
    await userEvent.upload(input, png());
    await waitFor(() => expect(FakeXhr.all).toHaveLength(1));
    FakeXhr.all[0]!.reply(503, apiErr("busy"));
    await waitFor(() => expect(FakeXhr.all).toHaveLength(2));
    expect(delays).toEqual([2000]);
    uploaded = true;
    FakeXhr.all[1]!.reply(200, created);
    expect(await screen.findByRole("img", { name: "Photo 1" })).toBeInTheDocument();
  });

  it("a rate limit stops the rest of the batch without sending it", async () => {
    const { input } = await openEditor();
    await userEvent.upload(input, [png("a.png"), png("b.png")]);
    await waitFor(() => expect(FakeXhr.all).toHaveLength(1));
    FakeXhr.all[0]!.reply(429, apiErr("rate_limited"), { "Retry-After": "60" });
    await waitFor(() => expect(screen.getAllByText(/Try again in 1 min/)).toHaveLength(2));
    expect(FakeXhr.all).toHaveLength(1);
  });

  it("sets and clears the cover and the profile photo", async () => {
    const withCover = yb({ cover_media_id: PHOTO }, { photo_media_id: PHOTO });
    const { calls } = await openEditor({
      [`GET /yearbook/get?id=${ID}`]: book(
        yb({ cover_media_id: PHOTO }, { photo_media_id: PHOTO }),
      ),
      [MEDIA]: mediaPage([PHOTO]),
      ["POST /yearbook/update"]: (c) => book({ ...withCover, ...(c.body as object) }),
      ["POST /yearbook/update-profile"]: book(yb()),
    });
    const cover = await screen.findByRole("button", { name: "Cover photo", pressed: true });
    await userEvent.click(cover);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Cover photo", pressed: false }),
      ).toBeInTheDocument(),
    );
    expect(calls.find((c) => c.path === "/yearbook/update")?.body).toEqual({
      id: ID,
      cover_media_id: null,
    });
    await userEvent.click(screen.getByRole("button", { name: "Profile photo", pressed: true }));
    await waitFor(() =>
      expect(calls.some((c) => c.path === "/yearbook/update-profile")).toBe(true),
    );
    expect(calls.find((c) => c.path === "/yearbook/update-profile")?.body).toMatchObject({
      yearbook_id: ID,
      full_name: "Lan Nguyễn",
      photo_media_id: null,
    });
  });

  it("deletes a photo after confirmation and reloads the book", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    let gone = false;
    const { calls } = await openEditor({
      [`GET /yearbook/get?id=${ID}`]: () => book(yb({ cover_media_id: gone ? null : PHOTO })),
      [MEDIA]: () => mediaPage(gone ? [] : [PHOTO]),
      "POST /media/delete": () => {
        gone = true;
        return Response.json({ status: "OK", data: {} });
      },
    });
    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("img")).not.toBeInTheDocument());
    expect(screen.getByText("No photos to show yet.")).toBeInTheDocument();
    expect(
      calls.some(
        (c) =>
          c.method === "POST" &&
          c.path === "/media/delete" &&
          (c.body as { id: string }).id === PHOTO,
      ),
    ).toBe(true);
  });

  it("lists the stored photos after a reload and pages with the cursor", async () => {
    const { calls } = await openEditor({
      [MEDIA]: mediaPage([PHOTO], "c2"),
      [`${MEDIA}&next_id=c2`]: mediaPage([OTHER]),
    });
    const first = await screen.findByRole("img", { name: "Photo 1" });
    expect(first).toHaveAttribute("src", `/api/media/get-content?id=${PHOTO}&variant=thumb`);
    await userEvent.click(screen.getByRole("button", { name: "Show more photos" }));
    expect(await screen.findByRole("img", { name: "Photo 2" })).toHaveAttribute(
      "src",
      `/api/media/get-content?id=${OTHER}&variant=thumb`,
    );
    expect(screen.queryByRole("button", { name: "Show more photos" })).not.toBeInTheDocument();
    expect(calls.at(-1)?.path).toBe(`/media/get-list?yearbook_id=${ID}&next_id=c2`);
  });

  it("shows a translated error with a retry when the list fails", async () => {
    let fail = true;
    await openEditor({
      [MEDIA]: () =>
        fail ? Response.json(apiErr("storage_error"), { status: 502 }) : mediaPage([PHOTO]),
    });
    expect(await screen.findByText(/Photo storage is not responding/)).toBeInTheDocument();
    expect(screen.queryByText("No photos to show yet.")).not.toBeInTheDocument();
    fail = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("img", { name: "Photo 1" })).toBeInTheDocument();
  });

  it("deleting one of two photos refreshes the list without a page reload", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    let gone = false;
    await openEditor({
      [MEDIA]: () => mediaPage(gone ? [OTHER] : [PHOTO, OTHER]),
      "POST /media/delete": () => {
        gone = true;
        return Response.json({ status: "OK", data: {} });
      },
    });
    await userEvent.click((await screen.findAllByRole("button", { name: "Delete" }))[0]!);
    await waitFor(() => expect(screen.getAllByRole("img")).toHaveLength(1));
    expect(screen.getByRole("img")).toHaveAttribute(
      "src",
      `/api/media/get-content?id=${OTHER}&variant=thumb`,
    );
  });
});
