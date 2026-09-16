/**
 * Step-up authentication (docs/adr/017).
 *
 * Three instance actions — wipe, export, and promoting someone to instance
 * admin — need a fresh passkey assertion rather than just a valid session.
 * The server answers those requests with 403 and a `code` telling us which
 * situation we are in, so the flow is: try the action, and if the server asks
 * for presence, prove it and try once more.
 *
 * A passkey is no longer the only proof (docs/adr/099). A recovery code
 * issued before the current sign-in opens the same window, which is what
 * lets somebody whose device cannot make a passkey confirm anything at all.
 */
import { api } from './api.js';
import {
  prepareRequestOptions,
  serializeAssertionResponse,
  passkeyErrorMessage,
} from './webauthn.js';

/** The server's 403 code meaning "confirm with your passkey". */
export const SUDO_REQUIRED = 'sudo_required';

/** The server's code meaning "you have no passkey to confirm with". */
export const PASSKEY_REQUIRED = 'passkey_required';

/** Thrown when the person has no passkey enrolled yet. */
export class PasskeyRequiredError extends Error {
  constructor() {
    super('This action needs a passkey. Enroll one in Security settings first.');
    this.code = PASSKEY_REQUIRED;
  }
}

/**
 * Thrown by stepUpWithRecoveryCode. `code` is the server's own reason —
 * 'no_recovery_codes', 'recovery_codes_too_new', 'invalid_code', or
 * 'rate_limited' for the 429 — so the dialog can say a different sentence
 * for each instead of printing one shrug.
 */
export class RecoveryCodeError extends Error {
  constructor(message, code) {
    super(message);
    this.code = code;
  }
}

/**
 * Burn one recovery code to open the same five-minute window a passkey
 * opens (docs/adr/099).
 *
 * The code must predate this sign-in: a batch the session itself could have
 * minted proves nothing, so the server refuses it with
 * 'recovery_codes_too_new'. That costs one sign-out the first time and
 * nothing afterwards.
 */
export async function stepUpWithRecoveryCode(code) {
  try {
    return await api('auth/step-up/recovery', { method: 'POST', body: { code } });
  } catch (err) {
    const reason = err?.data?.code
      || (err?.status === 429 ? 'rate_limited' : 'invalid_code');
    throw new RecoveryCodeError(err?.message || 'That recovery code could not be checked.', reason);
  }
}

/**
 * The dialog that asks for a recovery code, registered once by the app.
 *
 * It lives here rather than in each of the seven surfaces that confirm
 * something, because the alternative is seven copies of one flow drifting
 * apart. Registration is optional on purpose: with no provider, withStepUp
 * behaves exactly as it did before docs/adr/099 — it throws — so a surface
 * that never mounted the dialog, and every test, sees no change.
 *
 * The provider is called with `{ hasPasskey, reason }` and resolves with the
 * recovery code that opened the window, the string 'passkey' when a passkey
 * opened it, or null when the person dismissed the dialog. Anything non-null
 * means the window is open and the action can be retried.
 */
let stepUpPrompt = null;

/** Register (or, with null, unregister) the prompt provider. */
export function setStepUpPrompt(fn) {
  stepUpPrompt = typeof fn === 'function' ? fn : null;
}

/** The registered provider, or null. Exported for the dialog's own use. */
export function getStepUpPrompt() {
  return stepUpPrompt;
}

/**
 * Whether an error from api() is the server asking for step-up.
 */
function needsStepUp(err) {
  const code = err?.data?.code;
  return code === SUDO_REQUIRED || code === PASSKEY_REQUIRED;
}

/**
 * Ask the current session holder to touch their authenticator, and open the
 * confirmation window on success.
 */
export async function stepUp() {
  let options;
  try {
    options = await api('auth/step-up/begin', { method: 'POST' });
  } catch (err) {
    if (err?.data?.code === PASSKEY_REQUIRED) throw new PasskeyRequiredError();
    throw err;
  }

  let credential;
  try {
    credential = await navigator.credentials.get(prepareRequestOptions(options));
  } catch (err) {
    throw new Error(passkeyErrorMessage(err, 'stepup'));
  }
  if (!credential) throw new Error('Confirmation was cancelled.');

  return api('auth/step-up/finish', {
    method: 'POST',
    body: serializeAssertionResponse(credential),
  });
}

/**
 * Run an action, and if the server asks for presence, prove it and run the
 * action again. The action is passed as a thunk because it is retried.
 *
 * Only one retry: if the second attempt still comes back asking for step-up,
 * something is wrong and looping on the authenticator prompt would be worse
 * than surfacing the error.
 *
 * Two paths reach the recovery-code dialog (docs/adr/099): there is no
 * passkey on the account at all, and there is one but the ceremony failed —
 * the second is the device that cannot produce an assertion, which is the
 * case ADR 017 assumed away. Where no dialog is registered both paths throw
 * exactly what they threw before.
 */
export async function withStepUp(action) {
  try {
    return await action();
  } catch (err) {
    if (!needsStepUp(err)) throw err;

    if (err?.data?.code === PASSKEY_REQUIRED) {
      const noPasskey = new PasskeyRequiredError();
      if (!stepUpPrompt) throw noPasskey;
      const opened = await stepUpPrompt({ hasPasskey: false, reason: '' });
      if (!opened) throw noPasskey;
      return action();
    }

    try {
      await stepUp();
    } catch (stepErr) {
      if (!stepUpPrompt) throw stepErr;
      // "Enroll one in Security settings first" is the sentence this dialog
      // exists to replace, so it is not passed on as the reason.
      const noPasskey = stepErr instanceof PasskeyRequiredError;
      const opened = await stepUpPrompt({
        hasPasskey: !noPasskey,
        reason: noPasskey ? '' : stepErr?.message || '',
      });
      if (!opened) throw stepErr;
    }
    return action();
  }
}

/**
 * Current step-up state for the signed-in session: whether they hold a
 * passkey at all, whether a confirmation window is already open, and how
 * many recovery codes this session could confirm with right now
 * (`recovery_ready` — codes minted during this sign-in are not counted).
 *
 * Admin screens read this on load so a missing passkey is visible *before*
 * someone reaches for a button it blocks.
 */
export async function stepUpStatus() {
  try {
    return await api('auth/step-up');
  } catch {
    return { has_passkey: true, active: false, recovery_ready: 0 };
  }
}
