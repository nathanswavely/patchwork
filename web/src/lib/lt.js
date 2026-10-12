/**
 * Liquid Textile's behaviours, for Patchwork. The one module that imports
 * the vendored behaviors.js, so no component reaches into vendor/ and the
 * day Liquid Textile becomes a package only this import moves
 * (docs/adr/2026-10-11-the-design-system-is-liquid-textile-vendored.md).
 *
 * Step 4b brings the viewport: the media conditions Liquid Textile's CSS is
 * written against, so script decides "narrow" exactly where the stylesheet
 * does. Step 5 adds the rest (dialog, toast, menu, tabs, segmented, fold).
 */
export { media, viewport } from '../vendor/liquid-textile/behaviors.js';
