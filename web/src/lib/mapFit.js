// Framing the map around its patches.
//
// Leaflet's fitBounds picks the zoom at which the bounds fill the map's size
// less its padding. A map with no width yet (mounted in a container that is
// hidden, collapsed, or not laid out) has nothing left once the padding is
// taken, and fitBounds does not refuse: the scale goes negative and the zoom
// lands on the ceiling, centred off every marker. Showing the map later keeps
// that view, so the opening fit was spent on a frame nobody could see, and the
// basemap under it sat at street level over empty ground until the GL layer
// gave up and fell back to raster tiles.

// The margin kept between the outermost markers and the map's edge.
export const FIT_PADDING_PX = 24;

// insetRight (0-1) is the fraction of the width a floating pane covers, so
// markers fit into the visible part rather than under the pane.
export function fitPadding(width, insetRight = 0) {
  const padRight = Math.round((width || 0) * insetRight);
  return {
    paddingTopLeft: [FIT_PADDING_PX, FIT_PADDING_PX],
    paddingBottomRight: [FIT_PADDING_PX + padRight, FIT_PADDING_PX],
  };
}

// Whether a map of `size` ({x, y}, as map.getSize() reports it) has any area
// left to frame markers in once `padding` is taken.
export function hasRoomToFit(size, padding) {
  const w = size.x - padding.paddingTopLeft[0] - padding.paddingBottomRight[0];
  const h = size.y - padding.paddingTopLeft[1] - padding.paddingBottomRight[1];
  return w > 0 && h > 0;
}
