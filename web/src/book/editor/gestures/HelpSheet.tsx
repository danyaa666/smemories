import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { SHORTCUTS } from "./keys";

/** The keyboard help (T-160): every shortcut of the editor in a table. A region the student opens with the toolbar button or `?`; Escape or Close ends it. */
export function HelpSheet({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const box = useRef<HTMLElement>(null);
  useEffect(() => {
    const opener = document.activeElement; // Escape or Close gives the keyboard back to what opened the list (T-182)
    box.current?.focus();
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    };
  }, []);
  return (
    <section ref={box} tabIndex={-1} className="ed-help" aria-label={t("editor.helpTitle")}>
      <h2>{t("editor.helpTitle")}</h2>
      <table>
        <tbody>
          {SHORTCUTS.map((s) => (
            <tr key={s.id}>
              <th scope="row">{s.keys || t(`editor.helpKeys.${s.id}`)}</th>
              <td>{t(`editor.help.${s.id}`)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <button type="button" onClick={onClose}>
        {t("editor.helpClose")}
      </button>
    </section>
  );
}
