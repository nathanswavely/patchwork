/**
 * Step-up authentication (docs/adr/017).
 *
 * Three instance actions — wipe, export, and promoting someone to instance
 * admin — need a fresh passkey assertion rather than just a valid session.
 * The server answers those requests with 403 and a `code` telling us which
 * situation we are in, so the flow is: try the action, and if the server asks
 * for presence, prove it and try once more.
 */
import { api } from './api.js';
import { prepareRequestOptions, serializeAssertionResponse } from './webauthn.js';

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

  const credential = await navigator.credentials.get(prepareRequestOptions(options));
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
 */
export async function withStepUp(action) {
  try {
    return await action();
  } catch (err) {
    if (!needsStepUp(err)) throw err;
    if (err?.data?.code === PASSKEY_REQUIRED) throw new PasskeyRequiredError();
    await stepUp();
    return action();
  }
}

/**
 * Current step-up state for the signed-in session: whether they hold a
 * passkey at all, and whether a confirmation window is already open.
 *
 * Admin screens read this on load so a missing passkey is visible *before*
 * someone reaches for a button it blocks.
 */
export async function stepUpStatus() {
  try {
    return await api('auth/step-up');
  } catch {
    return { has_passkey: true, active: false };
  }
}
