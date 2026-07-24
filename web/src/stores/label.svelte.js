/**
 * The Label (docs/adr/023): the quilt's stewardship disclosure.
 *
 * One fetch shared by every consumer — the /label page, the shell footer,
 * and the onboarding panel. Public endpoint, readable logged out.
 */
import { api } from '../lib/api.js';

let label = $state(null);
let loaded = $state(false);

export function getLabel() { return label; }
export function isLabelLoaded() { return loaded; }

export async function loadLabel(force = false) {
  if (loaded && !force) return label;
  try {
    label = await api('label');
  } catch {
    label = { published: false };
  }
  loaded = true;
  return label;
}

/**
 * Format integer minor units as money. The backend never uses floats for
 * money (ADR 008 rule); division by 100 happens only at the display edge.
 */
export function formatMoney(minor, currency) {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: currency || 'USD',
    }).format((minor || 0) / 100);
  } catch {
    return `${((minor || 0) / 100).toFixed(2)} ${currency || ''}`.trim();
  }
}
