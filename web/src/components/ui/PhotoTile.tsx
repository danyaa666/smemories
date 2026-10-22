import type { ReactNode } from "react";
import { QueuedImg } from "../QueuedImg";

type Props = {
  src: string;
  alt: string;
  tape?: boolean;
  /** Shows a remove button on the photo; removeLabel is its accessible name. */
  onRemove?: () => void;
  removeLabel?: string;
  disabled?: boolean;
  /** Caption or actions under the photo, inside the frame. */
  children?: ReactNode;
};

/** A polaroid: white frame, optional strip of tape (decoration only), remove button. */
export function PhotoTile({ src, alt, tape, onRemove, removeLabel, disabled, children }: Props) {
  return (
    <figure className="photo-tile">
      {tape && <span className="tape" aria-hidden="true" />}
      <div className="photo-tile-frame">
        <QueuedImg src={src} alt={alt} />
        {onRemove && (
          <button
            type="button"
            className="photo-tile-remove"
            aria-label={removeLabel}
            disabled={disabled}
            onClick={onRemove}
          >
            <span aria-hidden="true">×</span>
          </button>
        )}
      </div>
      {children}
    </figure>
  );
}
