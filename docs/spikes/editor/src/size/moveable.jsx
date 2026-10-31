import { createRoot } from "react-dom/client";
import { MoveableLayer as L } from "../moveable.jsx";
createRoot(document.body).render(<L items={[]} setItems={() => {}} unit="centre" size="A5" />);
