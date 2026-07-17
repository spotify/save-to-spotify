# Save to Spotify — Onboarding Flow

This file defines the **first-run onboarding flow** for the `save-to-spotify` skill. When a user runs `save-to-spotify` for the first time (or has no shows yet), the agent follows this flow instead of the default interview in SKILL.md.

The goal: collapse the path from "never heard of this" to "listening on Spotify" into six steps with minimal friction.

---

## When to activate

Use this flow when **any** of these are true:

- The user just installed `save-to-spotify` (the install script hands off here)
- The user has zero shows (`save-to-spotify --json shows` returns an empty `shows` array)
- The user explicitly asks to get started, set up, make their first episode, or any phrasing that signals they want the guided experience — **even if they already have shows**

The explicit ask always wins over the shows check. Do not skip onboarding just because shows exist when the user asked for the guided flow.

---

## Step 0 · What is Save to Spotify?

When onboarding a user for the first time, do not assume the user is comfortable using an AI agent to generate audio content. Guide them through the process step-by-step, making it as clear and simple as possible. State upfront the capacity of the tool, what it actually does, and what tools (if any) users must provide, i.e. for voices. Be very clear that the default state of the content is private when it gets saved to a show in the user's Spotify Library, and it is not able to be shared.

Before installing or asking any questions, open with this intro:

> Save to Spotify helps you turn an idea into a personal audio episode in your Spotify Library. You choose the topic, shape the content, and pick a voice. It then helps create and save the episode to your Spotify Library. Episodes are not visible to other users. Content is not able to be shared.

Open with the intro as written, then move straight to Step 1 — don't front-load details. The specifics land where they're actionable: voice engines and what the user must provide come up in Step 2, examples of what they can create in Step 3.

---

## Step 1/6 · Install

**Goal:** CLI installed and on PATH.

```
curl -fsSL https://saveto.spotify.com/install.sh | bash
```

On Windows, run it in Git Bash. The installer downloads the binary, verifies it, and prints:
```
✓ Installed save-to-spotify v0.2.0 to /usr/local/bin
Next: save-to-spotify setup
```

Hand off to Step 2 automatically — do not stop and wait.

---

## Step 2/6 · Auth

**Goal:** Spotify account connected in one action.

Run `save-to-spotify setup` directly — do not ask the user to run it. The agent executes this itself:

```shell
save-to-spotify setup
```

This handles auth + TTS detection in one pass:
- Desktop: opens browser automatically for OAuth
- Headless/agent: auto-detects and uses `--no-browser` mode
- On success: "✓ Authenticated. Token saved."

If setup reports no TTS engine, **ask the user** before installing anything:

> No voice engine found. Want me to install **Kokoro** (free, local, ~340 MB, [Apache-2.0 licensed](https://raw.githubusercontent.com/hexgrad/kokoro/refs/heads/main/LICENSE))?
>
> - **Yes, install Kokoro**
> - **I'll set up a cloud key instead** (OpenAI / ElevenLabs)

If they accept, run `save-to-spotify tts setup`. If it fails because Python is missing or older than 3.10, **ask the user** before installing:

> Kokoro needs Python 3.10+ but your system has an older version. Want me to install a newer Python?
>
> - **Yes, install Python** — `brew install python@3.12` (macOS), `apt install python3` (Debian/Ubuntu), or `winget install Python.Python.3.12` (Windows), then retry `tts setup`
> - **I'll handle it myself** — tell them what's needed and move on

If they prefer a cloud key instead of Kokoro, tell them which env var to set and move on — the engine will be picked up in Step 5.

Hand off to Step 3 immediately.

---

## Step 3/6 · Pick content

**Goal:** Pick and go. Zero typing to first episode.

Present curated recipes as a picker — no blank-page cold start. See [recipes.md](recipes.md) for the full list.

This step doubles as the post-authorization welcome: welcome the user to the Save to Spotify experience and let the recipes themselves be the examples of what they can create — encourage and inspire them to try a variety of use cases. Do it in one turn with the picker, not as a separate message.

Say: "Welcome to Save to Spotify! What would you like to create?" and present the top 3–4 most relevant recipes as choices. Include "Suggest more ideas" as a final option.

**When the user picks a recipe, start generating immediately.** Use the recipe's default input — do not ask follow-up questions. Most things are defaulted:

- Content: recipe's default input
- Language: user's system locale
- Length: recipe default (briefings ~8min, deep dives ~8min, travel ~6min)
- Voice: determined in Step 5
- Show: auto-created from recipe name

If the user types preferences into any answer's free-text field (tone, angle, length — anything), apply them and go — no follow-up questions. The chapter overview in Step 4 remains the main steering gate.

**Cover image** — in the same picker turn as the recipe. Offer "**Generate one for me**" (default) or "**I have an image**" — never label the option "AI-generated":

> Cover image: **Generate one for me** (default) or do you have an image you'd like to use?

If they don't respond or pick the default, generate the cover. If they provide an image path or URL, use that. Don't belabor it — one question, move on.

Only ask for other input if the recipe absolutely requires it (travel guide needs a destination, meeting recap needs a transcript, deep dive needs a topic — for the deep dive, offer 3 personalized topic suggestions plus free text, per [recipes.md](recipes.md); the topic is the user's intent, never invent it silently).

---

## Step 4/6 · Script

**Goal:** Chapters approved, then the script written. Iterating on an outline is free — no wasted writing, no TTS credits, no regeneration.

### Gather context

Run the recipe's data-gathering step. Show what was found:

```
✓ Google Calendar: 3 meetings today
✓ GitHub: 4 PRs merged overnight, 1 CI failure on main
✓ Linear: 5 open tasks, 1 blocked on QA sign-off
```

### Chapter overview

Present a short overview of the chapters the episode will cover — do **not** write the full script yet, and never dump a full transcript on the user.

Embed the overview inside the choice prompt itself (design principle 1). Use a compact numbered list — no blank lines between items. Each entry has a bold chapter name and a short one-line summary. When the episode's content was auto-derived rather than user-stated (e.g. a daily briefing), open with a one-line source note so the default is transparent, not silent — "Built from your calendar and GitHub activity —". End with the question:

```
Chapter overview — Deep Dive: How Solar Panels Work (~8 min):

1. **The photovoltaic effect** — how sunlight becomes electricity
2. **From cell to grid** — panels, inverters, and what happens to extra power
3. **The economics** — why prices fell 90% and what payback looks like

Does this outline cover what you want to create?
```

Choices:
- **Looks good** — write the full script from the approved chapters, then move on to picking a voice
- **Adjust the chapters** — revise the overview; stay in this step until the user approves

Only after approval, write the full script (following [content-quality.md](content-quality.md)) and self-critique it. Do not touch TTS engines, voices, or audio generation until the chapters are approved.

---

## Step 5/6 · Voice & audio

**Goal:** Working TTS engine with zero friction, then generate everything.

### Pick a voice

With the chapters approved and the script written, resolve a TTS engine. If an engine was already confirmed or installed in Step 2, use it directly — skip the status check. Only if engine setup was deferred in Step 2 (e.g. the user said they'd set a cloud key), follow the provider-selection flow in [audio-providers.md](audio-providers.md) (configured default → existing API key → Kokoro as the free local fallback), starting from `save-to-spotify tts status --json`.

On Kokoro, start from the recipe's default voice (the `Kokoro voice` row in its [recipes.md](recipes.md) table) — the voices vary in quality and a mismatched voice for the content type is avoidable. The user can still pick another.

Generate a short voice preview so the user hears the voice before committing:

```shell
save-to-spotify tts test --engine <engine> --voice <voice>
```

This synthesizes a sample phrase and prints its path — it does not auto-play (audio suddenly playing catches people off guard). Then:

- Don't hand the user the raw `file://` path
- Wrap the sample in the **voice-preview player page** ([local-preview.md](local-preview.md), "Voice preview page") and offer to open it
- Pass `--play` only if the user explicitly asks to hear it in-session

**HARD STOP — present choices and wait for the user to respond before continuing. Do not generate the episode audio until the user picks an option:**

- **Sounds good, generate my episode** — generate audio for the approved script
- **Try another voice** — list voices with `save-to-spotify tts voices --engine <engine>`, let the user pick, then run `tts test` again
- **Try another engine** — show available engines, switch, preview again

If this engine and voice were already previewed and approved earlier in the session (e.g. the user is returning from a script-only revision), skip the preview and the hard stop — go straight to generation.

### Generate

Generate audio, cover image, and timeline from the approved script — the cover is independent of the audio, so generate them concurrently. Show what was produced:

```
✓ Audio generated (4m 12s, 3 chapters)
✓ Cover image created
✓ Timeline built (3 chapters, 2 images, 4 links)
```

Hand off to Step 6.

---

## Step 6/6 · Preview & save

**Goal:** Episode on Spotify, with an optional look at the finished thing first. Privacy clear, next step offered.

Run the preview interaction from [local-preview.md](local-preview.md): build the page and start the localhost server before asking anything, offer open-or-skip, and ask for the verdict once the page is open. Add two onboarding-specific choices to the offer:

- **Make it shorter** — trim the chapter outline (Step 4), then regenerate the script and audio (Step 5)
- **Change the voice** — try a different TTS voice (back to Step 5)

In this flow, "content approval" for preview edits means Step 4's chapter approval.

### Save

```
✓ Show created: Daily Briefings
  Uploading to Spotify...

✓ Episode saved (private)

  Show:     Daily Briefings
  Duration: 4m 12s
  Listen:   https://open.spotify.com/episode/...
  Processing — ready in about a minute.
```

Key behaviors:
- Show auto-created from recipe name
- Privacy stated proactively: "(private)"
- Readiness expectation set: "ready in about a minute"

Immediately offer the highest-retention next step:

Say: "What would you like to do next?" with choices:
- **Create another episode** — return to Step 3
- **I'm done for now** — exit gracefully

---

## Design principles

1. **Always present choices, never ask for free text** — every decision point must be a list of options the user can pick from. Never ask the user to type something. Open-ended text input breaks the flow and creates friction. If you need input, present smart options and let them pick. And embed whatever the user is judging (chapter list, preview URL, file link) **inside the choice prompt itself** — plain text printed before a choice popup can be hidden by it, leaving the user a question about content they never saw
2. **Content before audio** — chapters are approved and the script written before any voice or audio work. Iterating on text costs nothing; regenerating audio does
3. **Show, don't tell** — script summaries, voice previews, and a local browser preview of the finished episode
4. **Default everything** — language, length, voice, cover, show, and content topic all have smart defaults
5. **Iterate, don't restart** — editing a script doesn't require re-answering setup questions
6. **Preview before commit** — nothing is uploaded without user approval; offer the local browser preview before saving
7. **Momentum over completeness** — get to a working episode fast, refine later
