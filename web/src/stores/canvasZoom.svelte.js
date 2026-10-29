// The canvas's zoom buttons and the canvas they drive.
//
// The buttons stand in SocialShell's canvas chrome beside the Quilt/Map
// switch, and the canvas is whichever of QuiltCanvas and MapView is showing,
// so the two meet here rather than through props. One pair of buttons for
// both surfaces: the map used to wear Leaflet's own control in a corner of
// its own, and the quilt had none, so the same act lived in two places on
// one and nowhere on the other.

let driver = $state(null);
let canIn = $state(false);
let canOut = $state(false);
let hidden = $state(false);

/**
 * A canvas offers itself to the buttons. `zoom(direction)` takes 1 or -1.
 * Returns the detach, which only detaches this driver: switching surfaces
 * mounts the next canvas before the last one's cleanup runs.
 */
export function attachCanvasZoom(zoom) {
  const d = { zoom };
  driver = d;
  return () => {
    if (driver === d) {
      driver = null;
      canIn = false;
      canOut = false;
    }
  };
}

/** The canvas states which way it can still go, after every zoom. */
export function reportCanvasZoom(zoomIn, zoomOut) {
  canIn = zoomIn;
  canOut = zoomOut;
}

/**
 * The page puts the buttons away while its canvas is out of sight: on a
 * phone the List view covers the canvas, which stays mounted underneath.
 */
export function hideCanvasZoom(hide) {
  hidden = hide;
}

export function hasCanvasZoom() {
  return driver !== null && !hidden;
}

export function canZoomIn() {
  return canIn;
}

export function canZoomOut() {
  return canOut;
}

export function zoomCanvas(direction) {
  driver?.zoom(direction);
}
