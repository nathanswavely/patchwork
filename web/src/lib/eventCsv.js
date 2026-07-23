// CSV → event rows for bulk upload. Parsing happens in the browser on
// purpose: the server speaks only validated JSON, and the person doing
// the upload sees exactly what will be created before anything is.
//
// Times are interpreted in the uploader's timezone — a venue admin's
// spreadsheet says "7:00 PM" in venue time, and the admin is standing in
// it. The preview renders the interpretation so mistakes are visible.

/** Parse CSV text into an array of string arrays. Handles quoted
 * fields, escaped quotes, embedded newlines, CRLF, and a UTF-8 BOM. */
export function parseCsv(text) {
  const rows = [];
  let row = [];
  let field = '';
  let inQuotes = false;
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (inQuotes) {
      if (c === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        field += c;
      }
    } else if (c === '"') {
      inQuotes = true;
    } else if (c === ',') {
      row.push(field);
      field = '';
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++;
      row.push(field);
      field = '';
      if (row.length > 1 || row[0] !== '') rows.push(row);
      row = [];
    } else {
      field += c;
    }
  }
  row.push(field);
  if (row.length > 1 || row[0] !== '') rows.push(row);
  return rows;
}

// Header aliases: spreadsheets never agree on names.
const HEADER_ALIASES = {
  title: ['title', 'name', 'event', 'event name', 'event title', 'show'],
  date: ['date', 'start date', 'day'],
  time: ['time', 'start time', 'doors'],
  start: ['start', 'starts', 'starts_at', 'starts at', 'startdate', 'datetime', 'start datetime'],
  end: ['end', 'ends', 'ends_at', 'ends at', 'end time', 'end date'],
  location: ['location', 'venue', 'where', 'place', 'room'],
  description: ['description', 'details', 'notes', 'about', 'info'],
};

function mapHeaders(headerRow) {
  const map = {};
  headerRow.forEach((raw, i) => {
    const h = raw.trim().toLowerCase();
    for (const [field, aliases] of Object.entries(HEADER_ALIASES)) {
      if (aliases.includes(h) && !(field in map)) {
        map[field] = i;
        return;
      }
    }
  });
  return map;
}

// "7:00 PM", "7pm", "19:00" → {hours, minutes}, or null.
function parseTime(s) {
  const m = s.trim().match(/^(\d{1,2})(?::(\d{2}))?\s*(am|pm|a\.m\.|p\.m\.)?$/i);
  if (!m) return null;
  let hours = parseInt(m[1], 10);
  const minutes = m[2] ? parseInt(m[2], 10) : 0;
  const half = m[3] ? m[3][0].toLowerCase() : '';
  if (hours > 23 || minutes > 59) return null;
  if (half === 'p' && hours < 12) hours += 12;
  if (half === 'a' && hours === 12) hours = 0;
  if (!half && !m[2] && s.trim().length <= 2) return null; // bare "7" is ambiguous
  return { hours, minutes };
}

// "2026-07-25", "7/25/2026", "7/25/26", "July 25, 2026" → local Date at
// midnight, or null.
function parseDate(s) {
  s = s.trim();
  let m = s.match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/);
  if (m) return new Date(+m[1], +m[2] - 1, +m[3]);
  m = s.match(/^(\d{1,2})\/(\d{1,2})\/(\d{2,4})$/);
  if (m) {
    let year = +m[3];
    if (year < 100) year += 2000;
    return new Date(year, +m[1] - 1, +m[2]);
  }
  const d = new Date(s);
  if (!Number.isNaN(d.getTime()) && /[a-zA-Z]/.test(s)) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  }
  return null;
}

// A combined "7/25/2026 7:00 PM" or ISO datetime → local Date, or null.
function parseDateTime(s) {
  s = s.trim();
  const iso = new Date(s);
  if (/\d{4}-\d{2}-\d{2}T/.test(s) && !Number.isNaN(iso.getTime())) return iso;
  const m = s.match(/^(.+?)\s+(\d{1,2}(?::\d{2})?\s*(?:am|pm|a\.m\.|p\.m\.)?)$/i);
  if (m) {
    const date = parseDate(m[1]);
    const time = parseTime(m[2]);
    if (date && time) {
      date.setHours(time.hours, time.minutes);
      return date;
    }
  }
  const dateOnly = parseDate(s);
  return dateOnly; // midnight local; preview makes this visible
}

/** Turn parsed CSV rows into upload-ready events.
 * Returns { events, errors, headerMap } where errors is
 * [{row, message}] using 1-based data-row numbers. */
export function rowsToEvents(rows) {
  if (rows.length < 2) {
    return { events: [], errors: [{ row: 0, message: 'the file needs a header row and at least one event' }], headerMap: {} };
  }
  const map = mapHeaders(rows[0]);
  const errors = [];
  if (!('title' in map)) errors.push({ row: 0, message: 'no title column found (try "title", "name", or "event")' });
  if (!('start' in map) && !('date' in map)) {
    errors.push({ row: 0, message: 'no date column found (try "date" or "start")' });
  }
  if (errors.length) return { events: [], errors, headerMap: map };

  const cell = (row, field) => (field in map ? (row[map[field]] || '').trim() : '');
  const events = [];
  for (let i = 1; i < rows.length; i++) {
    const row = rows[i];
    const rowNum = i; // 1-based data rows: header is row 0
    const title = cell(row, 'title');
    if (!title && row.every((c) => !c.trim())) continue; // blank line
    if (!title) {
      errors.push({ row: rowNum, message: 'missing title' });
      continue;
    }

    let start = null;
    if (cell(row, 'start')) {
      start = parseDateTime(cell(row, 'start'));
    } else {
      start = parseDate(cell(row, 'date'));
      if (start && cell(row, 'time')) {
        const t = parseTime(cell(row, 'time'));
        if (t) {
          start.setHours(t.hours, t.minutes);
        } else {
          errors.push({ row: rowNum, message: `can't read the time "${cell(row, 'time')}"` });
          continue;
        }
      }
    }
    if (!start) {
      errors.push({ row: rowNum, message: `can't read the date "${cell(row, 'start') || cell(row, 'date')}"` });
      continue;
    }

    const event = { title, starts_at: start.toISOString() };
    const endRaw = cell(row, 'end');
    if (endRaw) {
      let end = parseDateTime(endRaw);
      const timeOnly = parseTime(endRaw);
      if (!end && timeOnly) {
        end = new Date(start);
        end.setHours(timeOnly.hours, timeOnly.minutes);
        if (end <= start) end.setDate(end.getDate() + 1); // past-midnight show
      }
      if (!end) {
        errors.push({ row: rowNum, message: `can't read the end "${endRaw}"` });
        continue;
      }
      if (end <= start) {
        errors.push({ row: rowNum, message: 'end is before start' });
        continue;
      }
      event.ends_at = end.toISOString();
    }
    if (cell(row, 'location')) event.location = cell(row, 'location');
    if (cell(row, 'description')) event.description = cell(row, 'description');
    events.push(event);
  }
  return { events, errors, headerMap: map };
}

export const TEMPLATE_CSV =
  'title,date,time,end,location,description\n' +
  'Opening Night,2026-09-12,7:00 PM,10:00 PM,Main Stage,First show of the season\n' +
  'Artist Talk,9/19/2026,6:30 PM,,Gallery,Free and open to all\n';
