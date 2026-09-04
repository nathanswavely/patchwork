/**
 * WebAuthn helpers for ArrayBuffer <-> base64url conversions.
 */

/**
 * Decode a base64url string to an ArrayBuffer.
 */
export function base64urlToBuffer(base64url) {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

/**
 * Encode an ArrayBuffer to a base64url string.
 */
export function bufferToBase64url(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/**
 * Prepare creation options received from the server for navigator.credentials.create().
 * Converts base64url fields to ArrayBuffers.
 */
export function prepareCreationOptions(options) {
  const publicKey = { ...options.publicKey };

  publicKey.challenge = base64urlToBuffer(publicKey.challenge);
  publicKey.user = {
    ...publicKey.user,
    id: base64urlToBuffer(publicKey.user.id),
  };

  if (publicKey.excludeCredentials) {
    publicKey.excludeCredentials = publicKey.excludeCredentials.map((c) => ({
      ...c,
      id: base64urlToBuffer(c.id),
    }));
  }

  return { publicKey };
}

/**
 * Prepare request options received from the server for navigator.credentials.get().
 * Converts base64url fields to ArrayBuffers.
 */
export function prepareRequestOptions(options) {
  const publicKey = { ...options.publicKey };

  publicKey.challenge = base64urlToBuffer(publicKey.challenge);

  if (publicKey.allowCredentials) {
    publicKey.allowCredentials = publicKey.allowCredentials.map((c) => ({
      ...c,
      id: base64urlToBuffer(c.id),
    }));
  }

  return { publicKey };
}

/**
 * Serialize a PublicKeyCredential (creation response) for sending to the server.
 */
export function serializeCreationResponse(credential) {
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      attestationObject: bufferToBase64url(credential.response.attestationObject),
      clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
    },
  };
}

/**
 * Serialize a PublicKeyCredential (assertion response) for sending to the server.
 */
export function serializeAssertionResponse(credential) {
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      authenticatorData: bufferToBase64url(credential.response.authenticatorData),
      clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
      signature: bufferToBase64url(credential.response.signature),
      userHandle: credential.response.userHandle
        ? bufferToBase64url(credential.response.userHandle)
        : null,
    },
  };
}

/**
 * Whether this browser can do WebAuthn at all. False in plain-HTTP contexts
 * and in the in-app webviews some messaging apps still open links in, where
 * the API is simply absent — worth knowing before offering a button that
 * cannot work.
 */
export function passkeysSupported() {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential;
}

/**
 * Turn a failed ceremony into something a person can act on.
 *
 * The browser's own text is written for whoever is reading a console, not for
 * whoever is trying to sign in: every ordinary outcome — no passkey on this
 * device, prompt dismissed, prompt timed out — arrives as one indistinguishable
 * NotAllowedError reading "The operation either timed out or was not allowed."
 * Printed verbatim on the sign-in page it reads as a broken site, which is how
 * an account that had simply never enrolled a passkey looked like an outage.
 *
 * `action` is 'login', 'enroll', or 'stepup' — the same DOMException means
 * different things depending on which ceremony raised it.
 */
export function passkeyErrorMessage(err, action = 'login') {
  if (!passkeysSupported()) {
    return 'This browser cannot use passkeys. Open the site in Chrome, Safari, or Firefox and try again.';
  }

  const name = err && err.name;

  switch (name) {
    case 'NotAllowedError':
      // The catch-all. Never claim which of the three it was.
      if (action === 'enroll') {
        return 'Passkey setup was cancelled or timed out. You can try again.';
      }
      if (action === 'stepup') {
        return 'Passkey check was cancelled or timed out. You can try again.';
      }
      return 'No passkey for this site was found on this device, or the prompt was dismissed. Sign in with an email link below, then add a passkey from Settings → Security.';
    case 'InvalidStateError':
      // create() only: the authenticator already holds a credential we excluded.
      return 'This device already has a passkey for your account.';
    case 'NotSupportedError':
    case 'ConstraintError':
      return 'This device cannot create the kind of passkey this site needs. Use an email link instead.';
    case 'SecurityError':
      return 'This page address does not match the one your passkey was created for.';
    case 'AbortError':
      return 'Passkey prompt was closed.';
    default:
      return (err && err.message) || 'Passkey request failed.';
  }
}
