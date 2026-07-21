// Fetch a public API endpoint from multiple quilt instances in parallel.
// Returns results tagged with their source instance URL.
export async function queryAllQuilts(quilts, path) {
  const promises = quilts.map(async (quilt) => {
    try {
      const res = await fetch(`${quilt.url}${path}`);
      if (!res.ok) return [];
      const data = await res.json();
      const items = Array.isArray(data) ? data : data.items || data.data || [];
      return items.map((item) => ({
        ...item,
        _source: quilt.url,
        _sourceTags: quilt.tags || [],
      }));
    } catch {
      return [];
    }
  });

  const results = await Promise.all(promises);
  return results.flat();
}

// Sort merged results by date (newest first), falling back to created_at.
export function sortByDate(items, field = 'created_at') {
  return [...items].sort((a, b) => {
    const da = a.starts_at || a[field] || '';
    const db = b.starts_at || b[field] || '';
    return db.localeCompare(da);
  });
}
