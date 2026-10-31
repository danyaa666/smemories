import { AssetSvg, assets } from "./index";

// Every asset of the pack, plain and tinted, with its id and licence. A review page for the pack (docs/assets.md), reached at /dev/assets in
// `npm run dev` only; it is not in the production build and not in any menu.
const TINT = "#c2410c";

export default function ContactSheet() {
  return (
    <main style={{ padding: 16, fontFamily: "system-ui, sans-serif" }}>
      <h1 style={{ fontSize: 20 }}>Asset pack: {assets.length} assets</h1>
      <ul
        style={{
          listStyle: "none",
          margin: 0,
          padding: 0,
          display: "grid",
          gap: 8,
          gridTemplateColumns: "repeat(auto-fill, minmax(190px, 1fr))",
        }}
      >
        {assets.map((a) => (
          <li
            key={a.id}
            data-testid="asset-cell"
            style={{ border: "1px solid #ccc", borderRadius: 6, padding: 8, background: "#fff" }}
          >
            <div
              style={{
                display: "flex",
                gap: 8,
                alignItems: "center",
                justifyContent: "space-around",
                minHeight: 72,
              }}
            >
              <AssetSvg id={a.id} size={72} label={a.label_en} />
              <AssetSvg id={a.id} size={72} tint={TINT} label={`${a.label_en} (tinted)`} />
            </div>
            <div style={{ fontSize: 11, marginTop: 4, wordBreak: "break-all" }}>
              <strong>{a.id}</strong>
              <br />
              {a.licence}
              {a.tintable ? " · tintable" : ""}
            </div>
          </li>
        ))}
      </ul>
    </main>
  );
}
