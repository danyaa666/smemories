# Prompt for Claude Design: SMemories web UI

Paste everything below the line into Claude Design. Attach the two book designs (temp1, temp2) and any logo if you have them.

---

Design the web UI for **SMemories**, a site where university students make a yearbook, collect notes from friends, and print it as a PDF. Tagline: "Keep the memories, in a book you can hold."

## Product in one minute
- **Who:** university and college students, 18+. Mostly on phones, some on laptops. Interface languages: **English and Vietnamese** (Vietnamese text is about 20% longer, so layouts must not break).
- **Owner (signed in):** creates a yearbook, fills in their profile page (name, photo, nickname, quote, hobbies, future plans), creates **shareable note links**, reviews the notes friends send, picks a template, previews the book and prints/saves it as PDF from the browser.
- **Contributor (a friend, no account):** opens a link, writes a message, adds up to 3 photos, gives a name and an optional nickname or relationship, sends. Must feel as easy as a chat message, takes under a minute, works one-handed on a phone.
- **Privacy:** books are private by default. The owner approves or hides every note before it can be printed.

## Feel
Warm, nostalgic, friendly, a keepsake, not a corporate dashboard and not a children's app. Think paper, tape, handwriting accents, soft shadows, rounded corners. Calm and uncluttered; one clear primary action per screen. Avoid purple gradients, stock-photo clichés and heavy illustration. A small handwritten or serif accent font for headings is welcome; body text stays a highly legible sans-serif. Photos are the stars: give them room.

## Deliverables
1. **Design system:** colour tokens (light theme first, dark optional), type scale, spacing, radii, shadows, buttons, text fields with inline errors, 6-digit code input, select, photo uploader with thumbnails, cards, badges (Pending, Approved, Hidden), toasts, empty states, skeleton loaders, language switcher (EN / VI). Give the tokens as CSS variables.
2. **Screens**, each at **375 px mobile** and **1280 px desktop**, with the states listed:
   - Landing/home (signed out) and home (signed in)
   - Register, Login, "Continue with Google"
   - Verify email: enter the 6-digit code, "send a new code" with a 60-second cooldown, wrong-code error
   - Forgot password, Reset password (code + new password)
   - **Account:** email, language, change password, and a **username** (unique public handle, default like `user-9x3k2m7q`, can be changed once every 7 days: show the rules, the "next change allowed on <date>" state and the error states taken / reserved / too soon)
   - **My yearbooks:** list/grid with cover thumbnail, empty state ("Start your first yearbook"), create button
   - **New yearbook:** title and a few choices
   - **Edit yearbook:** profile form (name, nickname, photo, quote, hobbies, future plans), photo library with upload progress, delete confirmation
   - **Friends' notes links:** create a link (label, optional deadline), copy/share button, list of links with status (active, expired, revoked, full) and a revoke action
   - **Notes inbox (moderation):** all notes with photos, filter by Pending / Approved / Hidden, approve, hide, delete, drag-to-reorder, bulk actions
   - **Public note form (no account):** friendly header showing whose book it is, message, up to 3 photos with previews, name, nickname/relationship, send; success screen; and the dead ends: link expired, link full, link revoked, upload failed, slow connection
   - **Book preview and print:** page-by-page preview of the book at **A5 (148 x 210 mm)**, template picker (show two templates: **"memory-book"** warm scrapbook style and **"navy-classic"** formal navy and gold, follow the attached designs), a clear "Print / Save as PDF" button, and short device guidance for Chrome, Safari and mobile
3. **Book page templates** (the printed pages, not the website): cover, student profile page, friends' notes page with 4 to 6 notes and photos, full-bleed photo page. Print-safe margins (at least 10 mm), readable at A5, text never on a busy part of a photo.
4. **Empty, loading and error states** for every list and form, and a global "offline / server problem" banner.
5. A short **handoff note**: component list, spacing rules, how the layout changes between 375 px and 1280 px, tab order for the forms.

## Constraints
- **Accessible, WCAG 2.1 AA:** text contrast 4.5:1, touch targets at least 44 px, visible focus ring, errors in text under the field (never colour alone), works with keyboard and screen readers, respects reduced motion.
- **Mobile first.** Every screen works at 375 px with no horizontal scroll and a 16 px side gutter.
- Real-looking sample content with invented names and photos (mix of Vietnamese and English names). No lorem ipsum.
- Tone of the copy: short, kind, plain. Buttons say what they do ("Send my note", "Approve", "Copy link").
- It will be built in React with plain CSS variables, so avoid effects that need heavy libraries.

Start with the design system and the **public note form** and **notes inbox** (the most important flows), show me those, then continue with the rest.
