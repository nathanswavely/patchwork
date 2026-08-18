# Mail sends from its own domain

Transactional email — hosted quilts' magic links, console notices,
billing receipts — sends from a dedicated mail domain with its own
SPF/DKIM/DMARC, separate from both the marketing site and the
*.quilt.host apex where customer content lives. If one hosted quilt's
mail is spam-flagged, the blast radius is the sending domain, never the
brand's or every customer's web reputation. Per-quilt from-addresses,
outbound rate limits per instance. Sending from the customer's own
domain is a later, optional upgrade.
