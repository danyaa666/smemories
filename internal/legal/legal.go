// Package legal holds the version of the Terms of Use and Privacy Policy that users accept (D-35, T-111).
package legal

// CurrentVersion is the version of the four texts in docs/legal. Changing a text means editing all four files, bumping their
// front matter `version` and this constant (legal_test.go fails otherwise); every user then re-accepts at the next request.
const CurrentVersion = "2026-10-21"
