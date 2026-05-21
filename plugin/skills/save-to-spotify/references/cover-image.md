# Cover Image

**Every show & every episode MUST have a cover image.** Never save without `--image`.

**Format:** JPG or PNG, max 1 MB, 1400x1400 square.

## Paths (priority order)

1. **User-provided** — resize to 1400x1400.
2. **AI-generated** — default when image generation tools are available.
3. **CDN artwork** — preferred fallback. Always available, always legible.
4. **Stock + composite** — Openverse photo with strong overlay.
5. **Gradient** — last resort. Uses a CDN image as background, not a plain colour.

**Fallthrough:** AI fails → CDN → Stock (Openverse → Picsum) → Gradient. Gradient cannot fail.

**Default background:** When no user or AI image is available, always use a CDN image (`uts-{01..20}.png`) as the base. Never render a cover with only a solid colour or plain gradient — CDN artwork is always available.

### Path 1: User-provided

Accept JPG/PNG at any aspect ratio. Reject if below 600x600 or corrupted. Crop to square, resize to 1400x1400, compress to <1 MB (JPG 90%). Add typography unless user opts out of text.

**Never:** apply filters, AI enhancement, or override with an agent-generated image.

### Path 2: AI-generated (default)

**Never render text with the model** — composite with Pillow afterwards.

**Prompt construction:** transform the user's topic into a specific visual concept. Never pass the raw topic.

| User topic | Agent prompt |
| --- | --- |
| "Daily podcast about cats" | "Editorial photograph of a single tabby cat curled asleep on a linen cushion, soft window light, muted warm palette, square composition, negative space in lower third, no text, no logos" |
| "Weekly Stockholm news briefing" | "Minimalist illustration of a Stockholm rooftop skyline at dusk, muted blue-grey palette, square composition, negative space in lower third, no text, no logos" |

**Every prompt must include:** a specific concrete subject (not a concept), a composition direction (top-down/close-up/wide), a palette descriptor, "square composition", "negative space in lower third", "no text, no logos".

**Style:** photorealistic or clean illustration only. No collage, 3D renders, faces, or AI-generated likenesses. Silhouettes/hands/anonymous angles OK.

**Never produce:** podcast-meta imagery (mics, headphones), stock cliches (handshakes, lightbulbs), neon/HDR, baked-in text or logos.

**Skip AI if:** topic is abstract, involves real named people, refers to events after the model's training cutoff, or user requested otherwise.

See [timeline.md](timeline.md) for DALL-E / Stable Diffusion code examples.

### Path 3: CDN artwork (preferred fallback)

Pre-designed base artwork with Pillow typography. No overlay needed. 20 variants (`uts-01.png` through `uts-20.png`), selected by hash of show name.

**CDN endpoint:** `https://save-to-spotify.spotifycdn.com/assets/uts-{01..20}.png`

### Path 4: Stock + composite

Fetch a topic-matched photo from Openverse (no API key required, anon: 100/day, 5/hr).

**Source:** `https://api.openverse.org/v1/images/?q={query}&page_size=5&aspect_ratio=square&license=cc0`

**Query construction:** transform topic into concrete search terms:

| User topic | Search query |
| --- | --- |
| "Daily news podcast" | `newspaper morning light` |
| "Yoga" | `yoga mat minimal` |
| "Arsenal winning PL" | `football stadium red` |

**Selection:** first result ≥800x800. Avoid images with text, logos, or people as primary subject. All results CC0 (public domain).

**Fallback:** if Openverse fails, use Picsum (`https://picsum.photos/seed/{topic_slug}/1400/1400`).

**Treatment:** crop to 1400x1400, apply **strong overlay** (bottom 60%, max alpha 230).

### Path 5: Gradient (last resort)

Falls back to a CDN image as background (same as Path 3). If CDN is also unreachable, generate a Pillow gradient. **Palette direction:** lighter top, darker bottom (text sits at bottom, must be dark there). A plain gradient should only appear when all network requests have failed.

## Typography

**Mandatory** on every cover (unless user opted out in Path 1). Always composited with Pillow. Never rely on AI text rendering.

**Default copy:** show name only. Add date/episode number only to disambiguate >1 episode per day.

### Constraints

- **One label only.** No subtitles, taglines, or descriptors.
- **Max 3 lines.** If title doesn't fit, shorten: drop articles ("The Daily Stockholm News Briefing" → "Stockholm News Briefing"), use short forms ("The History of the Roman Empire" → "Roman Empire"). Full title preserved in metadata. After saving, surface shortened title to user & offer to regenerate.
- **No widows.** Don't strand a single short word on its own line.

### Typesetting

- **Fill canvas proportionally.** Longest line spans 80-85% of canvas width (`(CANVAS - 2*MARGIN) * 0.85`). Text block ≤50% of canvas height (`CANVAS - MARGIN - CANVAS // 2` = 636px). Whichever constraint hits first determines size.
- **Min 100pt** for thumbnail legibility (170x170). Auto-shorten if title can't fit at min size.
- **Tight leading.** `line_height = font_size * 0.97`.
- **Bottom-left aligned** (bottom-right for RTL). 64px margin from edges. Text never crosses vertical centre.
- **Break on meaning.** Keep concepts together. Pick whichever split produces the most balanced line widths.

### Font & RTL

| Script | Font | Alignment |
| --- | --- | --- |
| Latin (default) | **Montserrat Bold** | bottom-left |
| Arabic | **Tajawal Bold** | bottom-right |
| Hebrew | **Noto Sans Hebrew Bold** | bottom-right |

All OFL-licensed Google Fonts. Downloaded and cached on first use (`~/.cache/save-to-spotify/fonts/`). Bold or heavier only. Never system defaults or decorative fonts.

**RTL detection:** check `unicodedata.bidirectional(ch) in ('R', 'AL', 'AN')`. If any character is RTL, use RTL font and right-alignment.

**No reshaper libraries.** Do NOT use `arabic_reshaper` or `python-bidi` — modern fonts handle shaping natively in Pillow. Reshaper libraries break letter connections.

### Colour & effects

- **White text only.** No accent colours, no exceptions.
- **No text effects.** No drop shadows, strokes, outlines, glows. Legibility comes from per-path background treatment.

### Background treatment per path

| Path | Treatment |
| --- | --- |
| AI-generated | Prompt reserves negative space. No overlay. |
| CDN artwork | Built-in legibility. No overlay. |
| Stock / User-provided | **Strong overlay: bottom 60%, alpha 0→230.** |
| Gradient | Darker bottom provides contrast. No overlay. |

## Pillow compositing recipe

```python
from PIL import Image, ImageDraw, ImageFont
import os, hashlib, unicodedata, urllib.request

CANVAS = 1400
MARGIN = 64
MAX_TEXT_WIDTH = int((CANVAS - 2 * MARGIN) * 0.85)
MAX_TEXT_HEIGHT = CANVAS - MARGIN - CANVAS // 2  # 636px
MIN_FONT_SIZE = 100
MAX_FONT_SIZE = 400
LEADING_FACTOR = 0.97

FONT_CACHE = os.path.join(os.path.expanduser("~"), ".cache", "save-to-spotify", "fonts")
FONTS = {
    "latin":  ("Montserrat-Bold.ttf",       "https://raw.githubusercontent.com/JulietaUla/Montserrat/master/fonts/ttf/Montserrat-Bold.ttf"),
    "arabic": ("Tajawal-Bold.ttf",           "https://raw.githubusercontent.com/google/fonts/main/ofl/tajawal/Tajawal-Bold.ttf"),
    "hebrew": ("NotoSansHebrew-Bold.ttf",    "https://raw.githubusercontent.com/google/fonts/main/ofl/notosanshebrew/NotoSansHebrew%5Bwght%5D.ttf"),
}

def is_rtl(text):
    return any(unicodedata.bidirectional(ch) in ('R', 'AL', 'AN') for ch in text)

def detect_script(title):
    for ch in title:
        if '؀' <= ch <= 'ۿ' or 'ݐ' <= ch <= 'ݿ': return "arabic"
        if '֐' <= ch <= '׿': return "hebrew"
    return "latin"

def get_font_path(title=""):
    os.makedirs(FONT_CACHE, exist_ok=True)
    fname, url = FONTS[detect_script(title)]
    path = os.path.join(FONT_CACHE, fname)
    if not os.path.exists(path):
        urllib.request.urlretrieve(url, path)
    return path

def load_font(size, title=""):
    return ImageFont.truetype(get_font_path(title), size)

def measure_line(font, text):
    b = font.getbbox(text)
    return b[2] - b[0], b[3] - b[1]

def _split_combos(words, n):
    if n == 1: yield [words]; return
    for i in range(1, len(words) - n + 2):
        for rest in _split_combos(words[i:], n - 1):
            yield [words[:i]] + rest

def break_lines(title, font):
    words = title.split()
    best, best_d = None, float("inf")
    for n in range(1, min(len(words), 3) + 1):
        for combo in _split_combos(words, n):
            lines = [" ".join(p) for p in combo]
            ws = [measure_line(font, l)[0] for l in lines]
            if max(ws) > MAX_TEXT_WIDTH: continue
            d = max(ws) - min(ws)
            if d < best_d: best_d, best = d, lines
    return best or [title]

def fit_title(title):
    for sz in range(MAX_FONT_SIZE, MIN_FONT_SIZE - 1, -2):
        font = load_font(sz, title)
        lines = break_lines(title, font)
        if len(lines) > 3: continue
        if max(measure_line(font, l)[0] for l in lines) > MAX_TEXT_WIDTH: continue
        lh = int(sz * LEADING_FACTOR)
        total = lh * (len(lines) - 1) + font.getbbox(lines[-1])[3]
        if total > MAX_TEXT_HEIGHT: continue
        return font, lines, sz
    f = load_font(MIN_FONT_SIZE, title)
    return f, break_lines(title, f), MIN_FONT_SIZE

def composite_title(img, title):
    draw = ImageDraw.Draw(img)
    font, lines, sz = fit_title(title)
    lh = int(sz * LEADING_FACTOR)
    total = lh * (len(lines) - 1) + font.getbbox(lines[-1])[3]
    y = max(CANVAS - MARGIN - total, CANVAS // 2)
    rtl = is_rtl(title)
    for line in lines:
        x = CANVAS - MARGIN - measure_line(font, line)[0] if rtl else MARGIN
        draw.text((x, y), line, font=font, fill=(255, 255, 255))
        y += lh
    return img

def strong_overlay(img):
    ov = Image.new("RGBA", img.size, (0, 0, 0, 0))
    d = ImageDraw.Draw(ov)
    start = int(CANVAS * 0.40)
    for y in range(start, CANVAS):
        d.line([(0, y), (CANVAS, y)], fill=(0, 0, 0, int((y - start) / (CANVAS - start) * 230)))
    return Image.alpha_composite(img.convert("RGBA"), ov).convert("RGB")

def fetch_openverse(query):
    import json
    url = f"https://api.openverse.org/v1/images/?q={query.replace(' ', '+')}&page_size=5&aspect_ratio=square&license=cc0"
    req = urllib.request.Request(url, headers={"User-Agent": "save-to-spotify/1.0"})
    data = json.loads(urllib.request.urlopen(req).read())
    for img in data.get("results", []):
        if img.get("width", 0) >= 800 and img.get("height", 0) >= 800:
            return img["url"]
    return None

PALETTES = [
    ((40,60,90),(10,10,30)), ((70,45,80),(20,12,25)),
    ((30,80,70),(5,20,20)),  ((80,55,30),(25,18,8)),
    ((55,55,65),(15,15,18)), ((75,30,30),(18,6,6)),
    ((35,70,45),(6,18,10)),  ((70,70,70),(20,20,20)),
]

def gradient_bg(title):
    h = int(hashlib.md5(title.encode()).hexdigest(), 16)
    top, bot = PALETTES[h % len(PALETTES)]
    img = Image.new("RGB", (CANVAS, CANVAS))
    d = ImageDraw.Draw(img)
    for y in range(CANVAS):
        t = y / CANVAS
        d.line([(0, y), (CANVAS, y)], fill=tuple(int(top[i]+(bot[i]-top[i])*t) for i in range(3)))
    return img
```

## QA checklist

Before passing any cover to `--image`, verify:

- **Format:** JPG/PNG, <1 MB, exactly 1400x1400.
- **Typography:** present, ≤3 lines, correct font/alignment per script, white, 64px margins, lower 50% only.
- **Overlay:** applied on stock/user paths (60%, alpha 230). No overlay on AI/CDN/gradient.
- **AI path:** prompt was agent-constructed, no faces/text/logos in image.
- **Stock:** CC0 only, no text/logos/people in source.

If any check fails, fall through to next path.
