// One module per kind so a book that uses no asset (or only another kind) never loads this kind's SVG text (index.tsx imports it on demand).
export default import.meta.glob<string>("./svg/frame.*.svg", {
  query: "?raw",
  import: "default",
  eager: true,
});
