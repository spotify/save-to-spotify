# Cover Image

**Every show and every episode MUST have a cover image.** Without it, the content looks unfinished in Spotify. Never save without `--image`.

**Format:** JPG or PNG, max 1 MB, square 1400x1400. Add on save with `--image cover.jpg`.

Use the approach the user chose during the interview. If they didn't specify, default to AI-generated.

## Option 1: AI-generated (recommended)

Generate a themed illustration using DALL-E or another image generator. This produces unique, visually exciting covers.

```
Prompt pattern: "A bold, minimal illustration of [episode theme],
square format, vibrant colors, no text, suitable as a podcast cover image"
```

After generating the image, use Pillow to add the title text on top (AI text rendering is unreliable — always add text with Pillow). See "Pillow compositing recipe" below for the full code, plus [timeline.md](timeline.md) for DALL-E and Stable Diffusion code examples.

## Option 2: Pillow compositing (fast fallback)

Overlay text on a background image. Good for recurring episodes where speed matters more than uniqueness. See "Pillow compositing recipe" below for the full code example.

**Background image — license and use a free stock photo.** This produces much better results than source content images or solid colors. Search for a photo that matches the episode mood or topic:

- **Unsplash** -- `https://api.unsplash.com/search/photos?query=[topic]&orientation=squarish` (free, requires API key from unsplash.com/developers)
- **Pexels** -- `https://api.pexels.com/v1/search?query=[topic]&orientation=square` (free, requires API key from pexels.com/api)
- **Picsum** -- `https://picsum.photos/1400/1400` (random, no key needed — useful as a last-resort fallback)

Pick a photo with strong visual interest (landscapes, textures, bold colors). Avoid generic office/stock-people images. Use licensed sources only. The photo does the heavy lifting — text is just a label on top.

## Option 3: User-provided

The user supplies their own image. Resize to 1400x1400 square if needed.

## Design rules (all options)

**Layout:**
- **Maximum 2 lines of text** — show name and date/subtitle. Less is more
- Place text at the **bottom** of the image with a gradient overlay fading from transparent (top) to dark (bottom). This looks more polished than a rectangular panel
- Alternatively, use a blurred strip or semi-transparent bar behind the text — never darken the entire image
- Leave breathing room — don't push text to the edges. Padding of 40-60px from the sides

**Typography:**
- **Minimum 100pt for the title** — it must be legible at 170x170px thumbnail size
- **No thin or light font weights** — always bold, heavy, or black
- Prefer expressive fonts over Helvetica. On macOS: `SF Pro Display Bold`, `Futura Bold`, `Avenir Black`, `Impact`. Pick a font that matches the mood of the content
- **High contrast** — white or bright text on dark backgrounds. Add a subtle drop shadow (`textcolor=(255,255,255)` with a black shadow offset by 2-3px) for extra pop
- Subtitle/date in a smaller size (40-50pt) and slightly muted color (e.g., `(200, 220, 255)`)

**Color:**
- Let the background photo set the mood — don't fight it with clashing text colors
- White text works on almost any photo when you have a gradient overlay
- For a branded look, pick one accent color and use it consistently across episodes

## Pillow compositing recipe

Treat Pillow compositing as a first-class fallback, not a last-ditch hack. If DALL-E, Stable Diffusion, or stock-image lookup fails mid-session, finish production with a simple Pillow cover image instead of blocking the episode.

Reliable fallback recipe:

- 1400x1400 canvas
- gradient or two-tone background
- one simple geometric/illustrative element
- large bold title at the bottom
- small subtitle/date line
- subtle shadow for legibility

### Compositing a cover with gradient overlay

Use a bottom gradient overlay for text — this looks more polished than a rectangular panel:

```python
from PIL import Image, ImageDraw, ImageFont

# Load and crop to square 1400x1400
bg = Image.open('background.jpg').convert('RGB')
w, h = bg.size
side = min(w, h)
bg = bg.crop(((w-side)//2, (h-side)//2, (w+side)//2, (h+side)//2))
bg = bg.resize((1400, 1400), Image.LANCZOS)

# Bottom gradient overlay (transparent at top, dark at bottom)
overlay = Image.new('RGBA', bg.size, (0, 0, 0, 0))
overlay_draw = ImageDraw.Draw(overlay)
for y in range(700, 1400):
    alpha = int((y - 700) / 700 * 200)  # fade from 0 to 200
    overlay_draw.line([(0, y), (1400, y)], fill=(0, 0, 0, alpha))

bg = bg.convert('RGBA')
bg = Image.alpha_composite(bg, overlay)
bg = bg.convert('RGB')

# Add text — bottom-aligned, bold, high contrast
draw = ImageDraw.Draw(bg)
# Prefer expressive fonts: SF Pro Display Bold, Futura Bold, Avenir Black, Impact
title_font = ImageFont.truetype("/System/Library/Fonts/Supplemental/Futura.ttc", 110)
sub_font = ImageFont.truetype("/System/Library/Fonts/Supplemental/Futura.ttc", 48)

# Drop shadow for extra pop
draw.text((702, 1202), "Show Name", font=title_font, fill=(0, 0, 0, 180), anchor='mb')
draw.text((700, 1200), "Show Name", font=title_font, fill='white', anchor='mb')

draw.text((700, 1280), "Episode Subtitle — April 2026", font=sub_font, fill=(200, 220, 255), anchor='mb')

bg.save('cover.jpg', quality=90)
```

### Resizing with ffmpeg

```shell
# Resize cover image to 1400x1400 square
ffmpeg -i cover.png -vf "scale=1400:1400" cover_resized.jpg
```

### Typography minimums

- **At a 1400px canvas** (scale proportionally for other sizes — these are floors, not targets):
  - Title: **100pt** (≈7% of canvas height) — must survive a 170x170 thumbnail
  - Subtitle / date line: **40pt** (≈3%) — anything smaller disappears at thumbnail size
  - Subtitle must be **≥40% of the title size**. If the title is 120pt, the subtitle is ≥48pt.
- **Don't auto-shrink to fit** a long title. Truncate, abbreviate, or wrap to 2 lines before dropping below the minimum. Never use `textlength` / `getbbox` loops that shrink the font to fit.
- **Bold, heavy, or black weights only** — no thin or light fonts.
