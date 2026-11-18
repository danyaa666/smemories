import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import i18n from "../../i18n";
import { SaveStatus } from "./SaveStatus";
import type { SaveState } from "./store";

// T-160 A2: a server error says the save failed and will be retried; "offline" is only for a request that could not be made.
beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(cleanup);

const say = (state: SaveState, dirty = true) => {
  const { container } = render(<SaveStatus state={state} dirty={dirty} />);
  return container.textContent;
};

describe("SaveStatus", () => {
  it("says 'Could not save, retrying' for a server error and 'Offline' only for a failed request", () => {
    expect(say("failing")).toBe("Could not save, retrying...");
    cleanup();
    expect(say("offline")).toMatch(/^Offline/);
  });
  it("has the words in Vietnamese too", async () => {
    await i18n.changeLanguage("vi");
    expect(say("failing")).toBe("Chưa lưu được, đang thử lại...");
    cleanup();
    expect(say("offline")).toMatch(/^Mất kết nối/);
  });
  it("idle and dirty reads saving, idle and clean says nothing", () => {
    expect(say("idle", true)).toBe("Saving...");
    cleanup();
    expect(say("idle", false)).toBe("");
  });
  it("announces only what the student has to act on: saving and saved are shown but not in the live region (T-182)", () => {
    const { container } = render(<SaveStatus state="saved" dirty={false} />);
    expect(container).toHaveTextContent("Saved");
    expect(screen.getByRole("status")).toHaveTextContent("");
    cleanup();
    render(<SaveStatus state="conflict" dirty />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "This book was changed in another tab or device.",
    );
  });
});
