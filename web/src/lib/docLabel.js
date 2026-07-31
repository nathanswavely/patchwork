/**
 * Human label for a governance doc named by its file.
 *
 * `target_doc` and `documents[].filename` carry whatever the doc is stored as:
 * `community-standards.md`, `governance-rules.json`, or a bare title for docs
 * that never had a file. The badges that render it are text-transform:
 * capitalize, so an extension left on the end reads as part of the name —
 * "To Tool Library Rules.Md". Strip the extension, not one known extension.
 */
export function docLabel(name) {
  return String(name || '').replace(/\.(json|md)$/i, '').replace(/-/g, ' ');
}
