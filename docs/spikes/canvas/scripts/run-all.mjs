// Every step in order. Needs: node 22, Go (the validator), poppler (pdftoppm, pdfinfo, pdffonts, pdftotext), Playwright's Chromium, and the Doory clone (npm run clone).
import { execFileSync } from "node:child_process";
const steps = [
  ["coverage (Q1, Q2)", "node", ["scripts/coverage.mjs"]],
  ["validate with internal/booklayout (Q2)", "sh", ["-c", "cd ../../.. && go run -tags spike ./docs/spikes/canvas/validate"]],
  ["determinism (Q2)", "node", ["scripts/determinism.mjs"]],
  ["renders (Q1)", "node", ["scripts/render.mjs"]],
  ["print parity (Q2)", "node", ["scripts/parity.mjs"]],
  ["studio bundle (Q4, Q7)", "node", ["scripts/bundle.mjs"]],
  ["studio failure modes (Q4)", "node", ["scripts/failures.mjs"]],
];
for (const [name, cmd, args] of steps) { console.log(`\n== ${name}`); execFileSync(cmd, args, { stdio: "inherit" }); }
