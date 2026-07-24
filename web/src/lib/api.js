/**
 * API client wrapper for Patchwork backend.
 * Prepends /api/v1/, sets headers, parses JSON, throws on errors.
 */
export async function api(path, options = {}) {
  const url = `/api/v1/${path.replace(/^\//, '')}`;
  const method = (options.method || 'GET').toUpperCase();

  const headers = {
    'Content-Type': 'application/json',
    ...(options.headers || {}),
  };

  // Add mutation header for non-GET requests
  if (method !== 'GET') {
    headers['X-Patchwork-Request'] = 'true';
  }

  const fetchOptions = {
    method,
    headers,
    credentials: 'same-origin',
  };

  if (options.body !== undefined && method !== 'GET') {
    fetchOptions.body = typeof options.body === 'string'
      ? options.body
      : JSON.stringify(options.body);
  }

  const res = await fetch(url, fetchOptions);

  // Handle 204 No Content
  if (res.status === 204) {
    return null;
  }

  let data;
  try {
    data = await res.json();
  } catch {
    if (!res.ok) {
      throw new Error(`Request failed: ${res.status} ${res.statusText}`);
    }
    return null;
  }

  if (!res.ok) {
    const message = data?.error || data?.message || `Request failed: ${res.status}`;
    const err = new Error(message);
    err.status = res.status;
    err.data = data;
    throw err;
  }

  return data;
}
