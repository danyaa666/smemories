import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import i18n from "../../i18n";
import { BookEnvProvider, Photo } from "../env";

// T-154: a layout image uses the `display` version for a background and skips itself when it cannot load.

const env = { size: "A5" as const, t: i18n.getFixedT("en"), eager: true, track: () => {} };
const wrap = (ui: React.ReactNode) => <BookEnvProvider value={env}>{ui}</BookEnvProvider>;

describe("Photo", () => {
  it("asks the 3000 px display version when told, the print version otherwise", async () => {
    const { container } = render(
      wrap(
        <>
          <Photo id="BG1" variant="display" quiet />
          <Photo id="PH1" />
        </>,
      ),
    );
    await waitFor(() => expect(container.querySelectorAll("img[src]")).toHaveLength(2));
    const srcs = [...container.querySelectorAll("img")].map((i) => i.getAttribute("src"));
    expect(srcs).toContain("/api/media/get-content?id=BG1&variant=display");
    expect(srcs).toContain("/api/media/get-content?id=PH1&variant=print");
  });

  it("a quiet photo draws nothing when its file is gone (the book's own photo draws an empty frame)", async () => {
    const { container } = render(wrap(<Photo id="GONE1" variant="print" quiet />));
    const img = await waitFor(() => {
      const i = container.querySelector("img[src]");
      expect(i).not.toBeNull();
      return i as HTMLImageElement;
    });
    // the first failure is retried once after a second (QueuedImg), the second one is final
    fireEvent.error(img);
    await new Promise((r) => setTimeout(r, 1300));
    await waitFor(() => expect(container.querySelector("img[src]")).not.toBeNull());
    fireEvent.error(container.querySelector("img") as HTMLImageElement);
    await waitFor(() => expect(container.querySelector("img")).toBeNull());
    expect(container.querySelector(".bk-ph")).toBeNull();
  }, 10000);

  it("an id-less quiet photo is nothing, a loud one is an empty frame", () => {
    const { container } = render(
      wrap(
        <>
          <Photo id={null} quiet />
          <Photo id={null} />
        </>,
      ),
    );
    expect(container.querySelectorAll(".bk-ph")).toHaveLength(1);
    expect(screen.queryByRole("img")).toBeNull();
  });
});
