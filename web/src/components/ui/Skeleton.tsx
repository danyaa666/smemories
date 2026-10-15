/** A grey placeholder for content that is loading. Hidden from screen readers: the screen announces the loading state itself. */
export function Skeleton({
  width,
  height = 16,
}: {
  width?: string | number;
  height?: string | number;
}) {
  return <span className="skeleton" aria-hidden="true" style={{ width, height }} />;
}
