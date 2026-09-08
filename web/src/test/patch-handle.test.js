/**
 * The verified atproto handle on the patch profile (docs/adr/062, amended
 * 2026-09-08). A claim proved a binding once; the page says so in those
 * terms and does no more than that.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { handleFromDID } from '../lib/atproto.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('handleFromDID', () => {
  it('reads the handle off a did:web', () => {
    expect(handleFromDID('did:web:tellus.example')).toBe('tellus.example');
  });

  it('takes the host only — later segments are path, not domain', () => {
    // did:web:example.com:orgs:tellus serves its document at
    // /orgs/tellus/did.json; the handle is still the domain.
    expect(handleFromDID('did:web:example.com:orgs:tellus')).toBe('example.com');
  });

  it('decodes a percent-encoded port, which belongs to the host', () => {
    expect(handleFromDID('did:web:localhost%3A3000')).toBe('localhost:3000');
  });

  it('gives nothing for a did:plc — ADR 062 accepts no other method', () => {
    // Decision 2 refuses did:plc outright, so one appearing in the column
    // is not a handle this page can vouch for.
    expect(handleFromDID('did:plc:ewvi7nxzyoun6zhxrhs64oiz')).toBe('');
  });

  it('gives nothing for empty, missing, or malformed values', () => {
    expect(handleFromDID('')).toBe('');
    expect(handleFromDID(null)).toBe('');
    expect(handleFromDID(undefined)).toBe('');
    expect(handleFromDID('did:web:')).toBe('');
    expect(handleFromDID('tellus.example')).toBe('');
  });
});

describe('PatchProfile: the handle is a fact in About', () => {
  const src = source('pages/PatchProfile.svelte');

  it('derives the handle from the DID, never from the domain column', () => {
    // verification_domain stays behind on a seamrip while did travels
    // (ADR 062 consequences), so a surface built on the domain would go
    // blank on the fork.
    expect(src).toContain("import { handleFromDID } from '../lib/atproto.js'");
    expect(src).toContain('handleFromDID(node?.did)');
    expect(src).not.toContain('verification_domain');
  });

  it('opens About for a patch whose only public fact is its handle', () => {
    const line = src.split('\n').find((l) => l.includes('let showAbout ='));
    expect(line).toBeTruthy();
    expect(line).toContain('atprotoHandle');
  });

  it('renders the handle as text, not as a link', () => {
    // There is no client this project can bless to send a visitor to.
    const block = src.slice(src.indexOf('{#if atprotoHandle}'));
    const handleBlock = block.slice(0, block.indexOf('{/if}'));
    expect(handleBlock).toContain('@{atprotoHandle}');
    expect(handleBlock).not.toContain('<a ');
    expect(handleBlock).not.toContain('href');
  });

  it('claims a past proof, not a present one — no badge, no checkmark', () => {
    // Nothing re-checks the binding after the claim, so present-tense
    // copy would promise a check that is not running.
    expect(src).toContain('proved when this patch was claimed');
    expect(src).not.toMatch(/Verified\s+handle/i);
  });
});
