# E09_T-160 — Editor gestures: move, resize, rotate, snap, guides, with keyboard control (web)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-159. **Read first:** `docs/adr/0004-layout-editor.md` (Q2 in full, risks 4, 8 and 9, "Changes this implies" 2), the prototype `docs/spikes/editor/src` (gesture code, 112 lines, proved exact on A5/A4/Letter), `web/src/book/layout/geometry.ts` (`place`, `unplace`), `web/src/book/editor/*` (T-158 `begin`/`preview`/`commit`), the T-159 selection overlay.

#### Why
Selecting and typing numbers (T-159) is not an editor. Students expect to drag, resize and rotate, and people who cannot use a mouse need the same power from the keyboard. ADR 0004 chose to write the gestures in-house (3 KB gzip against 82 KB for a library that also got the stage scale wrong), so this task owns that module.

#### Requirements (SHALL)
1. **Module** `web/src/book/editor/gestures/` (no new dependency). A gesture starts from the box **as drawn on the edited page** (`place`, clamped), adds the pointer delta converted with the stage's current scale (read from the page element's bounding box, so zoom and fit-to-window are exact), snaps, clamps, and writes back through `unplace` into the T-158 store: `begin()` at pointer down, `preview()` on every pointer move (rAF-throttled), one `commit()` at pointer up. Escape or a cancelled pointer restores the start box and records no step.
2. **Move**: drag the item (or a selected template element via its override). **Resize**: eight handles; rotation-aware (the resize projects the pointer onto the item's own axes); Shift keeps the aspect ratio; minimum size 2 % of the page width; the box may leave the page by at most half of its size (the server bound). **Rotate**: the rotate handle; Shift snaps to 15 degree steps; the angle is normalised to (-180, 180].
3. **Snap and guides**: snap to the page centre lines, the page edges, the manifest safe margin and the drawn edges/centres of the other items (threshold 6 screen px, zero-cost when no pointer is down: index the snap lines once at pointer down). Guides are drawn in the T-159 overlay (outside the page clip). Holding Alt (macOS: Option) while dragging disables snapping.
4. **Pointer handling**: Pointer Events with a pointerId filter, `setPointerCapture`, `touch-action` set so a swipe over an unselected item still pans the stage; on touch the first tap selects, a drag of the selected item moves it. **Hit targets are at least 44 x 44 screen px** for handles (the drawn handle may be smaller), at any zoom; on a page too small for that the handles sit outside the corners.
5. **Keyboard** (same code path as the pointer, so the same snap and clamp rules): arrow keys move 1 design px, Shift+arrow 10; resize with Ctrl+Alt+arrow or the documented alternative; rotate with `,` and `.` (and Shift for 15 degrees); Delete removes an item; Escape clears the selection; Tab cycles items. **Pick shortcuts that work on Windows, Linux and macOS**: `Alt+Arrow` is browser Back on Windows/Linux, `[` `]` need AltGr on some layouts, Ctrl+Arrow switches desktops on macOS, so test the chosen set on each (or document the check as manual and list it for the owner) and also expose visible buttons for rotate/size in the T-159 property panel. A **keyboard help sheet** (button and `?`) lists all shortcuts in `vi` and `en`.
6. **Announcements**: after a keyboard step the polite live region says what changed in words ("moved right 10, position 42.5 percent"); gestures announce once at commit.
7. **Performance**: dragging one item among 100 holds at least 30 fps under 4x CPU throttle (the spike measured 59.8); the module must not re-render the whole page per move (memoised items, as in the prototype). A test records the number.
8. **Tests**: unit tests for the maths (round trip `place`/`unplace` on A5, A4 and Letter at zoom 0.6, 1 and 1.5; rotation-aware resize; clamp; snap choice; Shift/Alt modifiers); Playwright `tools/e2e/editor-gestures.spec.mjs` in `scripts/e2e.sh`: drag, corner-resize and rotate give a pointer-to-item error of at most 1 px on all three papers and at two zoom levels, keyboard move/resize/rotate, undo of a gesture is one step, Escape cancels, an item flush to the page edge can be resized with the mouse (handle outside the clip), touch drag at 768 px in Chromium (CDP touch).
9. Nothing under `.team/`; no server change.

#### Acceptance criteria
- [ ] AC1 — The Playwright spec passes with pointer error at most 1 px on A5, A4 and Letter at two zoom levels (measured by the test, numbers in the PR).
- [ ] AC2 — Keyboard-only: select, move, resize, rotate, delete, undo work; the shortcut set was checked on macOS, and on Windows or Linux (or written up as a manual check with the exact keys for the owner).
- [ ] AC3 — One gesture is one undo step; a cancelled gesture leaves no step and no save.
- [ ] AC4 — 100-item drag at or above 30 fps under 4x CPU throttle (recorded).
- [ ] AC5 — Lint, prettier, tsc, vitest, `lint:i18n`, `check:api`, build, `make print-check`, `make e2e` green.
- [ ] AC6 — Touch: first tap selects, then drag moves (768 px, Chromium); a swipe over an unselected item pans. Firefox and WebKit touch are listed as unproven if not run.

#### Test plan
Dev: unit tests, the Playwright spec, a recorded fps number. QA: runs everything on the PR head, repeats the pointer exactness on Letter at a non-default zoom, tries rotated-item resize, flush-to-edge items, hostile cases (item dragged beyond the bound, 40 items overlapping, rapid Escape), and the keyboard shortcut set on the platforms it has (macOS at least).

#### Added after T-159 QA (2026-11-01, in scope of this task)
A1. **Hit areas follow the drawn order.** QA found that after a template element is moved over another one, clicking the overlap selects the element underneath because the overlay's hit areas keep the template order (a moved template element is now drawn above the template's own content and below added items, T-159 rework 1). The overlay's hit order and the selection by Tab must follow the drawn order (topmost first); add a test that moves A over B and clicks the overlap.
A2. **Save-status label for a server error.** After a 500 on save the status shows "Offline"; a 5xx or other server error must say the save failed and will be retried ("Could not save, retrying"), and "Offline" only when the request itself fails. Unit test on the state mapping in `SaveStatus`/`store.ts`; strings in vi and en.
