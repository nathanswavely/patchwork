/**
 * Quilt Layout Engine — Affinity-first placement with filler squares.
 *
 * All placement math uses discrete integer grid units.
 * Pixel coordinates are computed only at the final rendering step.
 *
 * Tile sizes: 1x1, 2x2, 3x3, 4x4 grid cells.
 * Tiles flex ±1 tier to maintain affinity clustering.
 * Filler squares fill remaining gaps.
 */

// --- DATA STRUCTURES ---

/**
 * @typedef {Object} Tile
 * @property {string} id
 * @property {Object} data - Original patch data from API
 * @property {number} idealSize - 1..4 grid units
 * @property {number} currentSize - Actual placed size (may flex ±1)
 * @property {boolean} isFiller
 * @property {{col: number, row: number}|null} gridPos
 */

/**
 * @typedef {Object} LayoutResult
 * @property {Tile[]} tiles - All placed tiles (community + filler)
 * @property {number} minCol
 * @property {number} minRow
 * @property {number} maxCol
 * @property {number} maxRow
 */

// --- GRID STATE ---

class Grid {
  constructor() {
    // Map<string, string> — "col,row" -> tile ID or null
    this.cells = new Map();
    // Set<string> — "col,row" coordinates that are empty and adjacent to occupied
    this.frontier = new Set();
  }

  key(col, row) {
    return `${col},${row}`;
  }

  isOccupied(col, row) {
    return this.cells.has(this.key(col, row));
  }

  /** Check if a size×size block starting at (col, row) is all empty. */
  checkFit(col, row, size) {
    for (let r = row; r < row + size; r++) {
      for (let c = col; c < col + size; c++) {
        if (this.isOccupied(c, r)) return false;
      }
    }
    return true;
  }

  /** Place a tile: mark cells occupied and update the frontier. */
  place(col, row, size, tileId) {
    // Mark cells as occupied.
    for (let r = row; r < row + size; r++) {
      for (let c = col; c < col + size; c++) {
        this.cells.set(this.key(c, r), tileId);
        // Remove from frontier if it was there.
        this.frontier.delete(this.key(c, r));
      }
    }

    // Add new frontier cells: empty cells adjacent to the newly placed block.
    for (let r = row - 1; r <= row + size; r++) {
      for (let c = col - 1; c <= col + size; c++) {
        // Skip interior cells.
        if (r >= row && r < row + size && c >= col && c < col + size) continue;
        // Skip diagonals (only edge-adjacent).
        const isEdge = (r >= row && r < row + size) || (c >= col && c < col + size);
        if (!isEdge) continue;
        if (!this.isOccupied(c, r)) {
          this.frontier.add(this.key(c, r));
        }
      }
    }
  }

  /** Count how many occupied cells are edge-adjacent to a size×size block at (col, row). */
  adjacencyScore(col, row, size) {
    let score = 0;
    // Top edge.
    for (let c = col; c < col + size; c++) {
      if (this.isOccupied(c, row - 1)) score++;
    }
    // Bottom edge.
    for (let c = col; c < col + size; c++) {
      if (this.isOccupied(c, row + size)) score++;
    }
    // Left edge.
    for (let r = row; r < row + size; r++) {
      if (this.isOccupied(col - 1, r)) score++;
    }
    // Right edge.
    for (let r = row; r < row + size; r++) {
      if (this.isOccupied(col + size, r)) score++;
    }
    return score;
  }
}

// --- AFFINITY ---

function buildAffinityLookup(links) {
  const map = {};
  for (const link of links) {
    if (!map[link.source]) map[link.source] = {};
    if (!map[link.target]) map[link.target] = {};
    map[link.source][link.target] = link.strength;
    map[link.target][link.source] = link.strength;
  }
  return map;
}

/**
 * Calculate the target origin for a tile based on its affinity neighbors.
 * Returns the weighted average grid position of its placed neighbors.
 */
function calculateTargetOrigin(tileId, placedTiles, affinityLookup) {
  let totalWeight = 0;
  let wx = 0, wy = 0;

  for (const placed of placedTiles) {
    if (placed.isFiller) continue;
    const aff = affinityLookup[tileId]?.[placed.id] || 0;
    if (aff <= 0) continue;
    const cx = placed.gridPos.col + placed.currentSize / 2;
    const cy = placed.gridPos.row + placed.currentSize / 2;
    wx += cx * aff;
    wy += cy * aff;
    totalWeight += aff;
  }

  if (totalWeight === 0) {
    // No affinity — target center of mass of all placed tiles.
    if (placedTiles.length === 0) return { col: 0, row: 0 };
    let cx = 0, cy = 0;
    for (const p of placedTiles) {
      cx += p.gridPos.col + p.currentSize / 2;
      cy += p.gridPos.row + p.currentSize / 2;
    }
    return { col: cx / placedTiles.length, row: cy / placedTiles.length };
  }

  return { col: wx / totalWeight, row: wy / totalWeight };
}

// --- TIER ASSIGNMENT ---

/**
 * Minimum activity a patch must have to earn each tile size.
 * Absolute floors, not percentiles: a big tile has to be earned, so a quiet
 * quilt stays uniformly small instead of inventing a "biggest" patch out of
 * a field of zeros.
 */
const SIZE_FLOORS = [
  { size: 4, minActivity: 24 },
  { size: 3, minActivity: 10 },
  { size: 2, minActivity: 3 },
];

/**
 * How many followers count as one member when sizing a tile.
 *
 * Followers are observers, not participants, so they must not size a patch the
 * way membership does. But an unclaimed patch cannot have members at all —
 * membership is impossible until it is claimed — and has no events either, so
 * ignoring followers outright pins every unclaimed patch at 1x1 forever. On a
 * directory-seeded quilt that is nearly every patch, and the quilt can never
 * develop texture. Counting followers at a discount lets real interest
 * register without letting it masquerade as participation.
 *
 * Integer division keeps activity a whole number so ties compare exactly.
 */
const FOLLOWERS_PER_MEMBER = 3;

/** Sizing activity for a patch, in member-equivalents. */
function patchActivity(p) {
  return (p.member_count || 0)
    + (p.event_count || 0)
    + Math.floor((p.follower_count || 0) / FOLLOWERS_PER_MEMBER);
}

/** Largest tile size this activity level has earned, ignoring the rest of the quilt. */
function earnedSize(activity) {
  for (const { size, minActivity } of SIZE_FLOORS) {
    if (activity >= minActivity) return size;
  }
  return 1;
}

/**
 * Largest tile size rank is allowed to hold, ignoring activity.
 * Caps scale with quilt size so a big quilt can show several 4x4s, while a
 * small one can't turn into a wall of them. This is only a ceiling — a patch
 * still has to clear the floor above to actually get there.
 */
function rankCap(rank, n) {
  if (rank < Math.max(1, Math.ceil(n * 0.04))) return 4;
  if (rank < Math.max(1, Math.ceil(n * 0.12))) return 3;
  if (rank < Math.ceil(n * 0.4)) return 2;
  return 1;
}

function assignTiers(patches) {
  const items = patches.map(p => ({
    id: p.id,
    data: p,
    activity: patchActivity(p),
    idealSize: 1,
    currentSize: 1,
    isFiller: false,
    gridPos: null,
  }));

  // Sort by activity to assign tiers, but return in original (API/affinity)
  // order. Ties break on id so ordering stays stable between loads.
  const byActivity = [...items].sort(
    (a, b) => b.activity - a.activity || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
  );
  const n = byActivity.length;

  // Competition ranking: everyone tied on activity shares the best rank in
  // their group, so equally active patches always get the same cap — and so
  // the same size. Without this the cap splits ties on sort order alone,
  // which is the arbitrariness this sizing pass exists to remove.
  let groupRank = 0;
  byActivity.forEach((item, i) => {
    if (i > 0 && item.activity !== byActivity[i - 1].activity) groupRank = i;
    item.idealSize = Math.min(rankCap(groupRank, n), earnedSize(item.activity));
    item.currentSize = item.idealSize;
  });

  // Flat-quilt floor: when nobody cleared the 2x2 activity floor, the whole
  // quilt is 1x1 — and at that size the tile chrome stops being proportional.
  // The `?` badge (max(14, s*0.12)) and the pillow seam (max(3, s*0.03)) both
  // clamp to their pixel floors, and the outline stroke is a flat constant, so
  // a minimum tile wears a badge at ~20% of its width instead of the intended
  // 12% and an outline that reads twice as heavy. Zoom-fit can't undo any of
  // that: it scales the rendered result, but these proportions are baked into
  // world geometry first.
  //
  // Promoting the whole quilt one tier lifts tiles past those floors so the
  // designed ratios take over. Every patch still moves together, so this does
  // not invent a "biggest" patch out of a field of zeros — the property the
  // absolute SIZE_FLOORS exist to protect. Footprint doubles, but the canvas
  // zoom-fits, so on-screen tile size is unchanged.
  if (items.length > 0 && items.every(item => item.idealSize === 1)) {
    for (const item of items) {
      item.idealSize = 2;
      item.currentSize = 2;
    }
  }

  return items;
}

// --- MAIN LAYOUT ---

/**
 * Run the affinity-first quilt layout.
 * @param {Object[]} patches - Patch data from API (in affinity order)
 * @param {Object[]} affinityLinks - [{source, target, strength}]
 * @param {Map} [fixedSizes] - Optional map of patch ID → grid size (overrides tier assignment)
 * @returns {LayoutResult}
 */
export function quiltLayout(patches, affinityLinks, fixedSizes) {
  if (patches.length === 0) {
    return { tiles: [], minCol: 0, minRow: 0, maxCol: 0, maxRow: 0 };
  }

  const grid = new Grid();
  const affinityLookup = buildAffinityLookup(affinityLinks);
  const tiles = assignTiers(patches);

  // Override sizes if fixedSizes provided (keeps tiles stable across filter changes).
  if (fixedSizes) {
    for (const tile of tiles) {
      const fixed = fixedSizes.get(tile.id);
      if (fixed !== undefined) {
        tile.idealSize = fixed;
        tile.currentSize = fixed;
      }
    }
  }
  const placed = [];
  let fillerCount = 0;

  // Sort queue: descending by size, then by affinity (largest first to avoid Swiss cheese).
  // But within same size tier, preserve API order (affinity-sorted).
  const queue = [...tiles];
  queue.sort((a, b) => b.idealSize - a.idealSize);

  // Place the first tile at (0, 0).
  const first = queue.shift();
  grid.place(0, 0, first.currentSize, first.id);
  first.gridPos = { col: 0, row: 0 };
  placed.push(first);

  // --- PLACEMENT LOOP ---
  const MAX_FILLER_RETRIES = 3;

  while (queue.length > 0) {
    // Pick the next tile: highest total affinity to placed tiles.
    let bestIdx = 0;
    let bestAffinity = -1;

    for (let i = 0; i < queue.length; i++) {
      let totalAff = 0;
      for (const p of placed) {
        if (p.isFiller) continue;
        totalAff += affinityLookup[queue[i].id]?.[p.id] || 0;
      }
      if (totalAff > bestAffinity) {
        bestAffinity = totalAff;
        bestIdx = i;
      }
    }

    const tile = queue.splice(bestIdx, 1)[0];
    const target = calculateTargetOrigin(tile.id, placed, affinityLookup);

    // Sort frontier by distance to target origin.
    const frontierArr = [...grid.frontier].map(k => {
      const [c, r] = k.split(',').map(Number);
      const dist = Math.sqrt((c - target.col) ** 2 + (r - target.row) ** 2);
      return { col: c, row: r, dist };
    });
    frontierArr.sort((a, b) => a.dist - b.dist);

    let didPlace = false;

    // Try ideal size, then flex down, then flex up.
    const sizesToTry = [tile.idealSize];
    if (tile.idealSize > 1) sizesToTry.push(tile.idealSize - 1);
    if (tile.idealSize < 4) sizesToTry.push(tile.idealSize + 1);

    for (const trySize of sizesToTry) {
      if (didPlace) break;

      for (const fp of frontierArr) {
        // Try all anchor points: the frontier cell could be any corner of the tile.
        const anchors = [];
        for (let dr = 0; dr < trySize; dr++) {
          for (let dc = 0; dc < trySize; dc++) {
            anchors.push({ col: fp.col - dc, row: fp.row - dr });
          }
        }

        let bestAnchor = null;
        let bestScore = -1;

        for (const anchor of anchors) {
          if (!grid.checkFit(anchor.col, anchor.row, trySize)) continue;
          const adjScore = grid.adjacencyScore(anchor.col, anchor.row, trySize);
          if (adjScore <= 0) continue; // Must touch at least one placed cell.

          // Score: adjacency (density) + closeness to target.
          const cx = anchor.col + trySize / 2;
          const cy = anchor.row + trySize / 2;
          const dist = Math.sqrt((cx - target.col) ** 2 + (cy - target.row) ** 2);
          const score = adjScore * 10 - dist;

          if (score > bestScore) {
            bestScore = score;
            bestAnchor = anchor;
          }
        }

        if (bestAnchor) {
          tile.currentSize = trySize;
          tile.gridPos = bestAnchor;
          grid.place(bestAnchor.col, bestAnchor.row, trySize, tile.id);
          placed.push(tile);
          didPlace = true;
          break;
        }
      }
    }

    if (!didPlace) {
      // Filler fallback: drop a small filler near the target, then retry the tile.
      if (fillerCount < patches.length * 2) { // Safety cap.
        // Find the closest frontier cell that fits a 1x1 filler.
        for (const fp of frontierArr) {
          if (grid.checkFit(fp.col, fp.row, 1) && grid.adjacencyScore(fp.col, fp.row, 1) > 0) {
            const filler = {
              id: `filler-${fillerCount++}`,
              data: null,
              idealSize: 1,
              currentSize: 1,
              isFiller: true,
              gridPos: { col: fp.col, row: fp.row },
            };
            grid.place(fp.col, fp.row, 1, filler.id);
            placed.push(filler);
            // Re-queue the tile to retry.
            queue.unshift(tile);
            break;
          }
        }
      } else {
        // Absolute fallback: force-place at minimum size at any frontier cell.
        for (const fp of frontierArr) {
          if (grid.checkFit(fp.col, fp.row, 1)) {
            tile.currentSize = 1;
            tile.gridPos = { col: fp.col, row: fp.row };
            grid.place(fp.col, fp.row, 1, tile.id);
            placed.push(tile);
            break;
          }
        }
      }
    }
  }

  // Compute bounding box.
  let minCol = Infinity, minRow = Infinity, maxCol = -Infinity, maxRow = -Infinity;
  for (const t of placed) {
    if (!t.gridPos) continue;
    minCol = Math.min(minCol, t.gridPos.col);
    minRow = Math.min(minRow, t.gridPos.row);
    maxCol = Math.max(maxCol, t.gridPos.col + t.currentSize);
    maxRow = Math.max(maxRow, t.gridPos.row + t.currentSize);
  }

  return { tiles: placed, minCol, minRow, maxCol, maxRow };
}
