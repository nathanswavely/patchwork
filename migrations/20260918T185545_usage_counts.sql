-- Counting visitors without watching anyone.
-- See docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md
--
-- Two tables of daily totals, and nothing finer. A page view is one row
-- incremented; a visitor is counted once a day from a hash the server
-- forgets at midnight. No row here names a person, an address, a browser,
-- or a moment: the day is the finest grain, and the query string is never
-- read. Both stay behind in a seamrip (this deployment's traffic, not the
-- community's record) and are deleted outright by the admin's Clear.

CREATE TABLE usage_days (
    day   TEXT NOT NULL,   -- YYYY-MM-DD, UTC
    path  TEXT NOT NULL,   -- a route pattern ('/patches/{slug}'), never a URL
    views INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, path)
);

CREATE TABLE usage_visitors (
    day      TEXT PRIMARY KEY,   -- YYYY-MM-DD, UTC
    visitors INTEGER NOT NULL DEFAULT 0
);
