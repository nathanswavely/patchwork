/**
 * Clustering for the community map — PROTOTYPE (hand-rolled side).
 *
 * Lancaster's patches sit 11–40 metres apart downtown: at the zoom that
 * frames them all, 15 of 33 markers occlude each other, and separating the
 * closest pair by one marker width would need street-level zoom. So the
 * map clusters, and a cluster is not a patch — it wears a neutral disc and
 * a count, never an identity color or a motif.
 *
 * Greedy, seeded by activity: the most prominent patch anchors a cluster,
 * so the marker you can click is the one the quilt would have sized
 * largest. Same measure the labels use for priority.
 */

/** Sizing activity, mirroring quiltLayout's patchActivity. */
export function patchActivity(p) {
  return (p.member_count || 0)
    + (p.event_count || 0)
    + Math.floor((p.follower_count || 0) / 3);
}

/**
 * Group nodes whose markers fall within `radiusPx` of each other at the
 * map's current zoom. Returns [{ lead, members, latlng }], where a group of
 * one is a plain marker and anything larger is a cluster.
 */
export function clusterNodes(map, nodes, radiusPx) {
  const points = nodes
    .filter((n) => n.latitude != null && n.longitude != null)
    .map((n) => ({ node: n, pt: map.latLngToLayerPoint([n.latitude, n.longitude]) }));

  // Heaviest first, so a cluster is anchored by its most prominent member
  // rather than by whatever order the API returned (which is by name).
  points.sort((a, b) => patchActivity(b.node) - patchActivity(a.node));

  const groups = [];
  const claimed = new Set();

  for (const seed of points) {
    if (claimed.has(seed)) continue;
    claimed.add(seed);
    const members = [seed.node];
    let sumLat = seed.node.latitude;
    let sumLng = seed.node.longitude;

    for (const other of points) {
      if (claimed.has(other)) continue;
      if (seed.pt.distanceTo(other.pt) > radiusPx) continue;
      claimed.add(other);
      members.push(other.node);
      sumLat += other.node.latitude;
      sumLng += other.node.longitude;
    }

    groups.push({
      lead: seed.node,
      members,
      // A cluster sits at its members' mean, a lone marker at its own point.
      latlng: members.length === 1
        ? [seed.node.latitude, seed.node.longitude]
        : [sumLat / members.length, sumLng / members.length],
    });
  }

  return groups;
}
