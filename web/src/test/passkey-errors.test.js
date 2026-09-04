import { describe, it, expect, afterEach } from 'vitest';
import { passkeyErrorMessage, passkeysSupported } from '../lib/webauthn.js';

// Every ordinary passkey outcome — no credential on this device, prompt
// dismissed, prompt timed out — arrives as one indistinguishable
// NotAllowedError. Printing the browser's own words for it put
// "The operation either timed out or was not allowed. See:
// https://www.w3.org/TR/webauthn-2/#sctn-privacy-considerations-client"
// on the sign-in page, where an account that had simply never enrolled a
// passkey read as a broken site.

const original = window.PublicKeyCredential;

function withWebAuthn(present, fn) {
  if (present) {
    window.PublicKeyCredential = original ?? function () {};
  } else {
    delete window.PublicKeyCredential;
  }
  try {
    return fn();
  } finally {
    if (original === undefined) delete window.PublicKeyCredential;
    else window.PublicKeyCredential = original;
  }
}

afterEach(() => {
  if (original === undefined) delete window.PublicKeyCredential;
  else window.PublicKeyCredential = original;
});

const domException = (name) => Object.assign(new Error('raw browser text'), { name });

describe('passkeyErrorMessage', () => {
  it('never repeats the browser\'s NotAllowedError text', () => {
    withWebAuthn(true, () => {
      for (const action of ['login', 'enroll', 'stepup']) {
        const msg = passkeyErrorMessage(domException('NotAllowedError'), action);
        expect(msg).not.toMatch(/timed out or was not allowed/);
        expect(msg).not.toMatch(/w3\.org/);
      }
    });
  });

  it('points a failed sign-in at the email link, the door that is open', () => {
    withWebAuthn(true, () => {
      const msg = passkeyErrorMessage(domException('NotAllowedError'), 'login');
      expect(msg).toMatch(/email link/i);
    });
  });

  it('does not tell someone enrolling to go sign in by email', () => {
    withWebAuthn(true, () => {
      const msg = passkeyErrorMessage(domException('NotAllowedError'), 'enroll');
      expect(msg).toMatch(/cancelled or timed out/i);
      expect(msg).not.toMatch(/email link/i);
    });
  });

  it('reports an unsupported browser ahead of any error name', () => {
    withWebAuthn(false, () => {
      const msg = passkeyErrorMessage(domException('NotAllowedError'), 'login');
      expect(msg).toMatch(/cannot use passkeys/i);
    });
  });

  it('names the already-enrolled case rather than a generic failure', () => {
    withWebAuthn(true, () => {
      expect(passkeyErrorMessage(domException('InvalidStateError'), 'enroll'))
        .toMatch(/already has a passkey/i);
    });
  });

  it('falls through to the message on non-WebAuthn errors', () => {
    withWebAuthn(true, () => {
      const msg = passkeyErrorMessage(new Error('no login session found'), 'login');
      expect(msg).toBe('no login session found');
    });
  });

  it('survives a null or undefined error', () => {
    withWebAuthn(true, () => {
      expect(passkeyErrorMessage(null, 'login')).toBeTruthy();
      expect(passkeyErrorMessage(undefined, 'enroll')).toBeTruthy();
    });
  });
});

describe('passkeysSupported', () => {
  it('is false where the API is absent', () => {
    withWebAuthn(false, () => expect(passkeysSupported()).toBe(false));
  });

  it('is true where it is present', () => {
    withWebAuthn(true, () => expect(passkeysSupported()).toBe(true));
  });
});
