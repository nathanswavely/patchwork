/**
 * What a contact item is, in one place (docs/adr/083).
 *
 * The kind is a fact about the item rather than the column it sits in, which
 * is what lets a surface render tel: and mailto: without guessing. Three
 * surfaces need that fact — the person card, the profile, and the share
 * picker in My Patches — and each of them used to carry its own copy of this
 * map. Adding a fifth kind meant finding three files, and the copies had
 * already drifted: the same item showed an icon in one place and a word in
 * another. One module, so a kind is added once.
 */
import { Phone, EnvelopeSimple, At, Note } from 'phosphor-svelte';

export const CONTACT_KIND_WORD = {
  phone: 'Phone',
  email: 'Email',
  handle: 'Handle',
  note: 'Note',
};

export const CONTACT_KIND_MARK = {
  phone: Phone,
  email: EnvelopeSimple,
  handle: At,
  note: Note,
};

/**
 * The link a kind dials or opens, or null where there is nothing to open.
 *
 * A phone is stripped to digits and a leading +, because the value is the
 * person's own prose ("+1 717 555 0100, Signal only" is how people fill in a
 * phone field) and a tel: target cannot carry the prose. A handle and a note
 * have no scheme to dial: they render as text.
 */
export function contactHref(item) {
  if (!item) return null;
  if (item.kind === 'phone') return `tel:${item.value.replace(/[^+\d]/g, '')}`;
  if (item.kind === 'email') return `mailto:${item.value}`;
  return null;
}
