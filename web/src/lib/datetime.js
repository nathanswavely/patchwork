// Conversions between the UTC instants the API stores in `starts_at` and the
// wall-clock strings an <input type="datetime-local"> reads and writes.
//
// A datetime-local value carries no zone, so both directions have to name the
// one they mean. Today that is the browser's zone, which is what every
// rendering surface in the app already assumes. docs/adr/045 replaces that
// assumption with the event's own zone; these two functions are the seam
// where that change lands, which is why the pairing lives here rather than
// inline in the form.

const pad = (n) => String(n).padStart(2, '0');

// UTC instant -> "YYYY-MM-DDTHH:MM" read in the browser's zone.
//
// Slicing the ISO string instead (`iso.slice(0, 16)`) reads UTC digits into a
// control that means local, which moved the event by the editor's offset on
// every save.
export function toLocalInputValue(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

// "YYYY-MM-DDTHH:MM" in the browser's zone -> UTC instant.
//
// A date-time form with no offset is parsed as local by spec, which is the
// inverse of the above; a date-only form would be parsed as UTC, so the
// caller must pass a value that carries a time.
export function fromLocalInputValue(value) {
  if (!value) return undefined;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}
