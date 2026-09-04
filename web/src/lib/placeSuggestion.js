// Asking the gazetteer where an address is (docs/adr/080).
//
// The answer is a *suggested placement*: a marker proposed for a person to
// look at and confirm. It is never a map location until they do, which is
// what keeps a placed point placed rather than derived.
//
// Every failure here is the same answer as a polite miss — null. An instance
// with no gazetteer, an address that is prose, a network blip: none of them
// is a problem the person typing an address should be told about, because in
// all three cases the picker works exactly as it always has.

import { api } from './api.js';
import { hasMapLocation } from './mapLocation.js';

// Below this, an address is a fragment. The index refuses one-token queries
// anyway, and asking on every keystroke of "La" would be noise.
const MIN_QUERY_LENGTH = 5;

export function worthLookingUp(address) {
  return (address || '').trim().length >= MIN_QUERY_LENGTH;
}

// suggestPlace returns { latitude, longitude, label } or null.
export async function suggestPlace(address) {
  const q = (address || '').trim();
  if (!worthLookingUp(q)) return null;
  try {
    const res = await api(`gazetteer/suggest?q=${encodeURIComponent(q)}`);
    if (!res?.found || !res.place) return null;
    const { latitude, longitude } = res.place;
    if (!hasMapLocation(latitude, longitude)) return null;
    return { latitude, longitude, label: res.label || '' };
  } catch {
    // A miss and an outage look the same from here, and should: the person
    // places the marker themselves either way.
    return null;
  }
}
