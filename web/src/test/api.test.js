import { describe, it, expect, vi, beforeEach } from 'vitest';
import { api } from '../lib/api.js';

describe('api client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('prepends /api/v1/ to paths', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({ data: 'test' }),
    });

    await api('nodes/tree');
    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/nodes/tree', expect.anything());
  });

  it('strips leading slashes from path', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({}),
    });

    await api('/nodes/tree');
    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/nodes/tree', expect.anything());
  });

  it('defaults to GET with Content-Type json', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({}),
    });

    await api('health');
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.method).toBe('GET');
    expect(opts.headers['Content-Type']).toBe('application/json');
    expect(opts.credentials).toBe('same-origin');
  });

  it('adds X-Patchwork-Request header for mutations', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({}),
    });

    await api('nodes', { method: 'POST', body: { name: 'test' } });
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.method).toBe('POST');
    expect(opts.headers['X-Patchwork-Request']).toBe('true');
  });

  it('does NOT add X-Patchwork-Request for GET', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({}),
    });

    await api('nodes');
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.headers['X-Patchwork-Request']).toBeUndefined();
  });

  it('serializes body as JSON for POST', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({ id: '123' }),
    });

    const body = { name: 'Gallery Row', description: 'Art galleries' };
    await api('nodes', { method: 'POST', body });
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.body).toBe(JSON.stringify(body));
  });

  it('does not send body for GET requests', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({}),
    });

    await api('nodes', { body: { ignored: true } });
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.body).toBeUndefined();
  });

  it('returns null for 204 No Content', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 204,
    });

    const result = await api('auth/logout', { method: 'POST' });
    expect(result).toBeNull();
  });

  it('parses JSON response', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({ items: [1, 2, 3] }),
    });

    const result = await api('nodes');
    expect(result).toEqual({ items: [1, 2, 3] });
  });

  it('throws on non-ok response with error message', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: false, status: 404,
      json: async () => ({ error: 'node not found' }),
    });

    await expect(api('nodes/nonexistent')).rejects.toThrow('node not found');
  });

  it('throws with status code on error', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: false, status: 401,
      json: async () => ({ error: 'authentication required' }),
    });

    try {
      await api('auth/me');
    } catch (e) {
      expect(e.status).toBe(401);
      expect(e.message).toBe('authentication required');
    }
  });

  it('handles non-JSON error responses', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: false, status: 500, statusText: 'Internal Server Error',
      json: async () => { throw new Error('not json'); },
    });

    await expect(api('broken')).rejects.toThrow('Request failed: 500 Internal Server Error');
  });
});
