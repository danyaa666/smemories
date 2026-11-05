# Editing flow: page by page, save, print (T-170)

How a student edits a whole book in the free-layout editor and gets back to the book. Code: `web/src/book/editor/EditorShell.tsx` (toolbar, Done, Print, deep link),
`PageStrip.tsx`, `web/src/pages/YearbookEdit.tsx` (the entry points), `web/src/components/useLeaveGuard.ts` (`release`). Tests: `EditorFlow.test.tsx`,
`web/src/pages/editor.test.tsx`, `tools/e2e/editor-flow.spec.mjs`.

## Entry points

The book manage page (`/yearbooks/:id`) has a "Design the pages" section for a template that supports free layout:
**Edit page by page** (opens the cover) and one link per page kind the editor shows: **Edit cover**, **Edit profile page**, **Edit notes pages**, **Edit back page**.
A hidden section (stage 1 customise screen) has no link, and neither has "notes" while the book has no notes.
The link is `/yearbooks/:id/editor?page=<kind>` (`cover`, `profile`, `notes`, `back`). Only the first page is taken from it: an unknown value, or a kind the book has no
page of (hidden, no notes), opens the cover. The editor does not rewrite the address when the student moves on.
The stage 1 screen and the editor still link to each other ("Free layout" there, "Back to customise" here).

## Page by page

**Previous page** / **Next page** in the toolbar, and **Page Up** / **Page Down** (listed in the help sheet; not used by the gesture keys of T-160, and ignored in
a number field or a list), step through the book's pages in print order (`design.sections`, hidden sections skipped, one step per notes page). The position text ("Page 2 of 4: Profile")
is in the toolbar and in the live region. The buttons use `aria-disabled` at the ends so a keyboard user keeps the focus. A page change clears the selection and keeps the undo history;
the page strip highlights and scrolls to the current page. All notes pages share one layout (ADR 0004).

## Done

**Done** runs `settle()` (the save in flight and the queued one). When the server has everything, the editor releases its leave guard, tells the route to refetch the book data
and goes to `/yearbooks/:id`. When the save failed (conflict, invalid layout, offline, server error) the editor stays open, the usual save status and banner show, and a message
says why it did not leave; nothing is lost and Done can be pressed again. Press Back on the manage page and the editor comes back, showing the saved layout.

The leave guard (T-148 pattern) is armed while anything is unsaved: closing the tab shows the browser's prompt, a click on an in-app link or the browser's Back opens the "Leave the editor?" dialog.
Done releases the guard's extra history entry and waits for the browser to finish popping it before it navigates, otherwise the pending Back would undo the navigation.

## Print

**Print or save as PDF** keeps the T-159 behaviour: the click opens a blank tab (so a popup blocker allows it), `settle()` runs, then the tab goes to `/yearbooks/:id/book`; if the save fails the tab
is closed and the message says so; with the popup blocked the route opens in a new window after the save. A line beside it names the page size and says the book prints all pages.
The editor tab stays as it was.

## Manual checks

Screen reader wording of the live region and focus after Done fails; the browser's Back button in Safari and Firefox with the guard armed; two tabs (409) on a real device.
