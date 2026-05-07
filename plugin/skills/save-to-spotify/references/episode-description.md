# Episode Description Format

Reference for building the HTML description that appears in the Spotify show-notes panel. Spotify auto-links `(M:SS)` timestamps for in-app seeking.

## Format

The description uses HTML `<p>` tags with timestamped entries on a single line (no literal newlines in the final string):

```html
<p>Summary of today's episode themes in 1-2 sentences.</p><p>(0:00) - Introduction</p><p>(0:18) - First segment title - <a href='https://example.com/article-1'>source</a></p><p>(1:42) - Second segment title - <a href='https://example.com/article-2'>source</a></p><p>(4:30) - Sign-off</p>
```

## Build it from the timeline

Read chapter entries from `timeline.json` (ignore image, link, and `spotify_entity` companions — those appear in the player, not the show notes):

```python
import json

timeline = json.load(open('timeline.json'))
chapters = [item['chapter'] for item in timeline['items'] if 'chapter' in item]
# source_links maps chapter title -> original article URL (from sourcing phase)
source_links = {"Segment title": "https://source.url/article"}

parts = ['<p>Summary of episode themes.</p>']
for ch in chapters:
    ms = ch['start_time_ms']
    ts = f"({ms // 60000}:{(ms % 60000) // 1000:02d})"
    title = ch['title']
    url = source_links.get(title)
    if url:
        parts.append(f"<p>{ts} - {title} - <a href='{url}'>source</a></p>")
    else:
        parts.append(f"<p>{ts} - {title}</p>")

description = ''.join(parts)
```

## Rules

1. Every entry wrapped in its own `<p>...</p>` block
2. Do NOT use `<br>` tags — they render as literal text on the Spotify desktop app
3. Timestamps as plain text `(M:SS)` in parentheses — Spotify auto-links these for seeking. Do NOT wrap timestamps in `<a>` tags
4. No leading zero on minutes, always two-digit seconds: `(0:05)`, `(1:42)`, `(12:30)`
5. External source links use HTML anchors: `<a href='URL'>source</a>`
6. Use single quotes inside `href` to avoid shell escaping issues
7. Entire description must be a single line with NO literal newlines
8. Start with a 1-2 sentence summary paragraph
9. Use `-` (hyphen) not `—` (em dash) to avoid shell encoding issues
10. Include Introduction and Sign-off entries (no source links for these)
11. Every entry with a known source URL MUST include the link — never fabricate URLs
12. `<b>`, `<i>` for emphasis (sparingly)

## Naming (per Spotify guidelines)

- **Show name:** Short, memorable, and searchable. Avoid generic names that get lost in search results
- **Episode title:** Include the date or episode number for recurring shows. Keep under ~60 characters so it doesn't truncate in the app
- **Description summary:** The first 1-2 sentences appear as a preview in podcast apps — front-load the most interesting hook, don't waste it on boilerplate
