/**
 * A recovery code confirms, not just a passkey (docs/adr/099).
 *
 * docs/adr/017 gated the irreversible on a fresh WebAuthn assertion and said
 * that locked nobody out, because enrolling needs only the session you hold.
 * It does: a device that cannot make a passkey could never perform a
 * step-up-gated action at all, and since ADR 017 the gate spread from three
 * instance acts to a patch's routine minute-taking (docs/adr/052). Worse,
 * the enrolment failure pointed people at an email link, which grants no
 * confirmation window and does not exist on an SMTP-less instance.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text; the lib functions are exercised directly.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { passkeyErrorMessage } from '../lib/webauthn.js';
import {
  withStepUp,
  setStepUpPrompt,
  stepUpWithRecoveryCode,
  PasskeyRequiredError,
  RecoveryCodeError,
} from '../lib/stepUp.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

/** An api() rejection, shaped the way lib/api.js shapes one. */
function apiError(message, { status = 403, code } = {}) {
  const err = new Error(message);
  err.status = status;
  err.data = code ? { error: message, code } : { error: message };
  return err;
}

const sudoRequired = () => apiError('confirm to continue', { code: 'sudo_required' });
const passkeyRequired = () => apiError('needs a passkey', { code: 'passkey_required' });

afterEach(() => {
  setStepUpPrompt(null);
  vi.unstubAllGlobals();
});

describe('stepUpWithRecoveryCode — the server says which refusal it is', () => {
  function stubFetch(status, body) {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: status >= 200 && status < 300,
      status,
      statusText: String(status),
      json: async () => body,
    })));
  }

  it('posts the code to the step-up recovery endpoint', async () => {
    stubFetch(200, { active: true, codes_remaining: 7, used_recovery_code: true });
    const res = await stepUpWithRecoveryCode('aaaa-bbbb-cccc');
    expect(res.codes_remaining).toBe(7);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/v1/auth/step-up/recovery');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ code: 'aaaa-bbbb-cccc' });
  });

  it.each([
    ['no_recovery_codes'],
    ['recovery_codes_too_new'],
    ['invalid_code'],
  ])('surfaces the server code %s on the thrown error', async (code) => {
    stubFetch(400, { error: 'nope', code });
    await expect(stepUpWithRecoveryCode('x')).rejects.toMatchObject({ code });
    await expect(stepUpWithRecoveryCode('x')).rejects.toBeInstanceOf(RecoveryCodeError);
  });

  it('calls the 429 rate_limited, which carries no code of its own', async () => {
    stubFetch(429, { error: 'too many attempts. Wait a couple of minutes' });
    await expect(stepUpWithRecoveryCode('x')).rejects.toMatchObject({ code: 'rate_limited' });
  });
});

describe('withStepUp — with no prompt registered, nothing changes', () => {
  it('still throws PasskeyRequiredError when the account has no passkey', async () => {
    const action = vi.fn(async () => { throw passkeyRequired(); });
    await expect(withStepUp(action)).rejects.toBeInstanceOf(PasskeyRequiredError);
    expect(action).toHaveBeenCalledTimes(1);
  });

  it('still rethrows a failed passkey ceremony', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({ error: 'begin failed' }),
    })));
    const action = vi.fn(async () => { throw sudoRequired(); });
    await expect(withStepUp(action)).rejects.toThrow(/begin failed/);
    expect(action).toHaveBeenCalledTimes(1);
  });

  it('passes an unrelated error straight through', async () => {
    const action = vi.fn(async () => { throw apiError('not found', { status: 404 }); });
    await expect(withStepUp(action)).rejects.toThrow('not found');
  });
});

describe('withStepUp — with a prompt registered, a code opens the window', () => {
  it('asks the prompt when the account has no passkey, then retries once', async () => {
    const prompt = vi.fn(async () => 'aaaa-bbbb-cccc');
    setStepUpPrompt(prompt);
    let calls = 0;
    const action = vi.fn(async () => {
      calls += 1;
      if (calls === 1) throw passkeyRequired();
      return 'done';
    });
    await expect(withStepUp(action)).resolves.toBe('done');
    expect(prompt).toHaveBeenCalledTimes(1);
    expect(prompt.mock.calls[0][0]).toMatchObject({ hasPasskey: false });
    expect(action).toHaveBeenCalledTimes(2);
  });

  it('asks the prompt when the passkey ceremony fails, and says why', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 500,
      statusText: 'Server Error',
      json: async () => ({ error: 'authenticator unavailable' }),
    })));
    const prompt = vi.fn(async () => 'aaaa-bbbb-cccc');
    setStepUpPrompt(prompt);
    let calls = 0;
    const action = vi.fn(async () => {
      calls += 1;
      if (calls === 1) throw sudoRequired();
      return 'done';
    });
    await expect(withStepUp(action)).resolves.toBe('done');
    expect(prompt).toHaveBeenCalledTimes(1);
    expect(prompt.mock.calls[0][0].reason).toMatch(/authenticator unavailable/);
    expect(action).toHaveBeenCalledTimes(2);
  });

  it('does not retry when the person dismisses the prompt', async () => {
    const prompt = vi.fn(async () => null);
    setStepUpPrompt(prompt);
    const action = vi.fn(async () => { throw passkeyRequired(); });
    await expect(withStepUp(action)).rejects.toBeInstanceOf(PasskeyRequiredError);
    expect(action).toHaveBeenCalledTimes(1);
  });
});

describe('StepUpPrompt — each refusal gets its own sentence', () => {
  const src = source('components/StepUpPrompt.svelte');

  it('registers itself as the app-wide prompt and unregisters on teardown', () => {
    expect(src).toMatch(/setStepUpPrompt\(ask\)/);
    expect(src).toMatch(/return \(\) => setStepUpPrompt\(null\)/);
  });

  it('branches on every server code, and on the rate limit', () => {
    for (const code of ['no_recovery_codes', 'recovery_codes_too_new', 'rate_limited']) {
      expect(src).toContain(`case '${code}':`);
    }
    // invalid_code is the default: a wrong code and an unlabelled refusal
    // read the same to whoever typed it.
    expect(src).toMatch(/default:\s*\n\s*error = 'That recovery code is not right, or it has already been used\./);
  });

  it('tells the too-new case the one thing that fixes it', () => {
    expect(src).toMatch(/made during this sign-in/);
    expect(src).toMatch(/Sign out and sign back in using one of your codes/);
  });

  it('sends the codeless case to Settings → Security, and warns of the sign-out', () => {
    expect(src).toMatch(/This account has no recovery codes\./);
    expect(src).toMatch(/Make a set at Settings → Security\./);
  });

  it('offers the passkey only when there is one to offer', () => {
    expect(src).toMatch(/\{#if hasPasskey\}/);
    expect(src).toMatch(/Use a passkey/);
  });

  it('says how many codes are left, and stops for two or fewer', () => {
    expect(src).toMatch(/remaining <= 2/);
    expect(src).toMatch(/recovery codes left\./);
    expect(src).toMatch(/That was your last recovery code\./);
  });

  it('is dismissable and lands the cursor in the field', () => {
    // Escape and the backdrop both route through Modal's onClose.
    expect(src).toMatch(/<Modal \{open\} onClose=\{dismiss\}/);
    expect(src).toMatch(/el\.focus\(\)/);
  });
});

describe('App mounts the prompt once, for every surface', () => {
  const src = source('App.svelte');

  it('imports and renders StepUpPrompt alongside Toast', () => {
    expect(src).toMatch(/import StepUpPrompt from '\.\/components\/StepUpPrompt\.svelte'/);
    expect(src).toMatch(/<StepUpPrompt \/>/);
  });
});

describe('passkeyErrorMessage no longer promises an email link it cannot keep', () => {
  // passkeyErrorMessage reports an unsupported browser ahead of any error
  // name, and jsdom has no PublicKeyCredential — so the API has to be
  // present for these messages to be reached at all.
  const originalPKC = window.PublicKeyCredential;
  beforeEach(() => {
    window.PublicKeyCredential = originalPKC ?? function () {};
  });
  afterEach(() => {
    if (originalPKC === undefined) delete window.PublicKeyCredential;
    else window.PublicKeyCredential = originalPKC;
  });

  it('points an unsupported device at a recovery code, not at email', () => {
    for (const action of ['stepup', 'enroll']) {
      for (const name of ['NotSupportedError', 'ConstraintError']) {
        const msg = passkeyErrorMessage(Object.assign(new Error('raw'), { name }), action);
        expect(msg).toMatch(/recovery code/i);
        expect(msg).not.toMatch(/email link/i);
      }
    }
  });

  it('keeps enrolment and confirmation as different acts', () => {
    const enroll = passkeyErrorMessage(Object.assign(new Error('raw'), { name: 'NotSupportedError' }), 'enroll');
    const stepup = passkeyErrorMessage(Object.assign(new Error('raw'), { name: 'NotSupportedError' }), 'stepup');
    expect(enroll).toMatch(/cannot create/i);
    expect(stepup).toMatch(/Confirm with a recovery code/i);
    expect(enroll).not.toBe(stepup);
  });

  it('offers a recovery code where a cancelled confirmation used to dead-end', () => {
    const msg = passkeyErrorMessage(Object.assign(new Error('raw'), { name: 'NotAllowedError' }), 'stepup');
    expect(msg).toMatch(/recovery code/i);
  });

  it('still names the email link on the sign-in page, where it is a real door', () => {
    const msg = passkeyErrorMessage(Object.assign(new Error('raw'), { name: 'NotAllowedError' }), 'login');
    expect(msg).toMatch(/email link/i);
    expect(msg).toMatch(/recovery code/i);
  });
});

describe('Settings → Security says what a fresh batch can and cannot do yet', () => {
  const src = source('pages/SecuritySettings.svelte');

  it('warns that new codes confirm only after the next sign-in', () => {
    expect(src).toMatch(/confirm sensitive actions only after you sign in again/);
  });

  it('says a code confirms, not only signs in', () => {
    expect(src).toMatch(/A code also\s*\n?\s*confirms actions that can't be undone/);
  });
});

describe('PasskeyNotice stops sending the passkey-less at a passkey', () => {
  const src = source('components/PasskeyNotice.svelte');

  it('names the recovery code as the other way to confirm', () => {
    expect(src).toMatch(/a recovery code confirms too/);
  });
});
