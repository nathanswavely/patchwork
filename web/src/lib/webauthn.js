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
