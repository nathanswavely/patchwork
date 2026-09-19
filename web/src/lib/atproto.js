/**
 * Reading an atproto handle off a verified DID (docs/adr/062).
 *
 * A patch's handle is the domain its claim was verified against, and the
 * DID recorded by that claim is always a `did:web` on the same domain —
 * `did:web:tellus.example` for the handle `tellus.example`. The handle is
 * derived here rather than read from `verification_domain` because the DID
 * is the half that travels: a forked patch keeps `nodes.did` and loses the
 * old instance's vetting judgement, so a surface built on the domain column
 * would go blank on exactly the fork seamrip exists to make possible.
 */

/**
 * The handle a `did:web` names, or '' for anything else.
 *
 * Extra colon-separated segments are path, not host (the did:web method),
 * and a percent-encoded port belongs to the host — the same reading
 * `internal/atproto.DocURLFor` does on the server. Anything that is not a
 * `did:web` returns '', which is the whole of the display rule: ADR 062
 * accepts no other method, so a value that isn't one is not a handle this
 * page can vouch for.
 */
export function handleFromDID(did) {
  const s = String(did || '').trim();
  if (!s.startsWith('did:web:')) return '';
  const host = s.slice('did:web:'.length).split(':')[0];
  if (!host) return '';
  return host.replace(/%3A/gi, ':');
}
