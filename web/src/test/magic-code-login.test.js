/**
 * The sign-in code on the "Check your email" state.
 *
 * A magic link finishes sign-in in whatever session opens it — the browser
 * the mail client hands the URL to. When the mail opens somewhere else (a
 * phone, a client that has no browser of its own), that session is not this
 * one, and the person is stuck looking at a page that can only wait. The same
 * email now carries a six-digit code, and typing it here finishes the sign-in
 * in the tab that asked.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text (CLAUDE.md, "Verifying frontend changes").
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Vitest runs with the web/ project root as cwd.
function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the code form on the sent-email state', () => {
  const src = source('pages/Login.svelte');

  it('offers the code under the "Check your email" text', () => {
    expect(src).toContain('Check your email');
    expect(src).toContain('Or enter the code from the email');
    expect(src).toContain('id="signin-code"');
  });

  it('asks for the code the way a one-time code is asked for', () => {
    // A numeric keypad on a phone, the platform's own code autofill, and a
    // pattern that tolerates the space the email prints for reading.
    expect(src).toContain('inputmode="numeric"');
    expect(src).toContain('autocomplete="one-time-code"');
    expect(src).toMatch(/pattern="\[0-9 ]\*"/);
  });

  it('posts email and code to the verify endpoint', () => {
    expect(src).toContain("api('auth/magic-link/verify'");
    expect(src).toMatch(/body: \{ email, code: signInCode \}/);
    expect(src).toMatch(/method: 'POST'/);
  });

  it('sends a first-time address to username selection with the signup token', () => {
    // docs/adr/013: an address with no account gets a signup token, not an
    // account named after the email.
    expect(src).toContain("res?.status === 'username_required'");
    expect(src).toContain("'/signup/complete?token=' + encodeURIComponent(res.signup_token)");
    expect(src).toContain("'&redirect=' + encodeURIComponent(redirectTo)");
  });

  it('signs an existing account in and honours the redirect', () => {
    expect(src).toMatch(/await login\(\);\s*\n\s*navigate\(redirectTo\);/);
  });

  it('shows the refusal under the form, like the other forms do', () => {
    expect(src).toContain('{#if codeError}');
    expect(src).toContain('<p class="error-text">{codeError}</p>');
  });

  it('keeps the "Use a different email" way out, and clears the code with it', () => {
    expect(src).toContain('Use a different email');
    expect(src).toMatch(/emailSent = false;[^}]*signInCode = '';/);
  });

  it('disables the button while a code is in flight or nothing is typed', () => {
    expect(src).toContain('disabled={codeLoading || !signInCode.trim()}');
  });
});
