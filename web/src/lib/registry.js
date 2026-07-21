// Load quilts from a ?registry= URL parameter.
export async function loadRegistry(registryUrl) {
  const res = await fetch(registryUrl);
  if (!res.ok) throw new Error(`Registry fetch failed: HTTP ${res.status}`);
  const data = await res.json();
  if (!data.quilts || !Array.isArray(data.quilts)) {
    throw new Error('Invalid registry format: missing quilts array');
  }
  return data;
}
