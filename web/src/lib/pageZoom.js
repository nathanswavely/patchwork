/**
 * Keep a pinch inside a pan/zoom surface (the quilt, the map) from zooming the
 * page itself.
 *
 * `touch-action: none` is enough on Chrome/Android, but iOS Safari still
 * fires its non-standard gesture events and scales the whole document — so a
 * two-finger zoom on the quilt ends up blowing up the UI chrome around it.
 * Preventing gesturestart on the surface stops that without taking browser
 * zoom away from the rest of the page (no `user-scalable=no`).
 *
 * Returns a cleanup function; call it on component teardown.
 */
export function blockPageZoom(el) {
  if (!el) return () => {};
  const stop = (event) => event.preventDefault();
  el.addEventListener('gesturestart', stop);
  el.addEventListener('gesturechange', stop);
  el.addEventListener('gestureend', stop);
  return () => {
    el.removeEventListener('gesturestart', stop);
    el.removeEventListener('gesturechange', stop);
    el.removeEventListener('gestureend', stop);
  };
}
