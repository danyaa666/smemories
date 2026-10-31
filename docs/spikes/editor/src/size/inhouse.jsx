import { createRoot } from "react-dom/client";
import { InHouseLayer as L } from "../inhouse.jsx";
createRoot(document.body).render(<L items={[]} setItems={() => {}} unit="centre" size="A5" />);
