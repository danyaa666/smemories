type Props = {
  message: string;
  /** An optional single action, e.g. "Undo". It is a real button, so the keyboard reaches it. */
  actionLabel?: string;
  onAction?: () => void;
};

/** A transient message. The caller decides when to show and remove it. */
export function Toast({ message, actionLabel, onAction }: Props) {
  return (
    <div className="toast" role="status">
      <span>{message}</span>
      {actionLabel && onAction && (
        <button type="button" className="toast-action" onClick={onAction}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}
