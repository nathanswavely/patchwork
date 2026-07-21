-- Add links JSON array to nodes for external links (website, social, etc.)
-- Format: [{"url": "https://...", "label": "Website"}, ...]
ALTER TABLE nodes ADD COLUMN links TEXT DEFAULT '[]';
