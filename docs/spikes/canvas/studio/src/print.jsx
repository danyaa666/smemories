// The print route: the book only. It must never reach the studio chunk (bundle.mjs checks the build manifest).
import { createRoot } from "react-dom/client";
import { Book } from "./book.jsx";
createRoot(document.getElementById("root")).render(<Book />);
