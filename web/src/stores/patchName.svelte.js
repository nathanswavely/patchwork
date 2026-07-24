/**
 * Stores the current patch name for the work-mode header.
 * PatchShell writes this, App.svelte reads it.
 */
let name = $state('');

export function setPatchName(n) { name = n; }
export function getPatchName() { return name; }
