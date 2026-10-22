import { useEffect, useRef, useState, type ImgHTMLAttributes } from "react";
import { enqueue, loaded } from "./imageQueue";

const RETRY_MS = 1000;

type Props = Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "loading"> & {
  src: string;
  alt: string;
  /** Do not wait for the image to come near the screen (the book while the print dialog opens). */
  eager?: boolean;
};

/**
 * A gallery image (T-116): it asks for its file only when it is near the screen (like `loading="lazy"`) and a slot of the shared
 * queue is free, so a page of 100 photos never has more than `LIMIT` requests in flight. A failure is retried once after a second
 * (a 429 under load, which <img> cannot see) before `onError` runs.
 */
export function QueuedImg({ src, alt, eager, onLoad, onError, ...rest }: Props) {
  const ref = useRef<HTMLImageElement>(null);
  const [seen, setNear] = useState(typeof IntersectionObserver === "undefined");
  const near = seen || !!eager;
  const [turn, setTurn] = useState({ src: loaded.has(src) ? src : "", attempt: 0 });
  const release = useRef<() => void>(() => {});

  useEffect(() => {
    const el = ref.current;
    if (near || !el) return;
    const io = new IntersectionObserver(
      (entries) => entries.some((e) => e.isIntersecting) && setNear(true),
      { rootMargin: "300px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [near]);

  useEffect(() => {
    if (!near || loaded.has(src)) return;
    return enqueue((done) => {
      release.current = done;
      setTurn((t) => ({ ...t, src }));
    });
  }, [near, src, turn.attempt]);

  return (
    <img
      {...rest}
      alt={alt}
      ref={ref}
      src={loaded.has(src) ? src : turn.src === src ? src : undefined}
      onLoad={(e) => {
        loaded.add(src);
        release.current();
        onLoad?.(e);
      }}
      onError={(e) => {
        release.current();
        if (turn.attempt === 0) {
          setTimeout(() => setTurn({ src: "", attempt: 1 }), RETRY_MS);
          return;
        }
        onError?.(e);
      }}
    />
  );
}
