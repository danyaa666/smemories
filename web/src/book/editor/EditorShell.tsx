import type { ComponentProps } from "react";
import { EditorBody } from "./EditorBody";
import { Stage } from "./Stage";

/**
 * The free-layout editor with the desktop stage (drag, resize, rotate). The route (`pages/Editor.tsx`) uses `EditorBody` and loads the stage by itself, so that a
 * phone never fetches the gesture code (T-181); this one is for 768 px and wider and for the tests of that shell.
 */
export function EditorShell(props: Omit<ComponentProps<typeof EditorBody>, "Stage">) {
  return <EditorBody {...props} Stage={Stage} />;
}
