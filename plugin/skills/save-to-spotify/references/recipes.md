# Recipes

Ready-to-use content templates for the onboarding flow. Each recipe is fully defaulted — the agent can produce a complete episode without asking the user anything beyond "what would you like to create?"

When the user picks a recipe, **do not ask the input question**. Use the default input instead. The user can always refine after hearing the result. The goal is zero typing to first episode.

Each recipe carries a **default Kokoro voice** in its table below — use it as the first suggestion when the user is on Kokoro (they can still pick another at the voice step). The rows are the single source of truth for which voice fits which recipe.

---

## Daily briefing

**Description:** Your calendar, tasks, and repo activity as a short morning brief.

**Default input:** Pull from whatever is available — Google Calendar, GitHub, Linear. Auto-detect connected services.
**Input question (only if user asks to customize):** "What accounts or services should I pull from?"

| Default | Value |
|---------|-------|
| Length | ~8 min |
| Segments | 3–4 (schedule, repo, tasks, optional weather) |
| Show name | Daily Briefings |
| Kokoro voice | af_heart |

**Data sources:** Google Calendar API, GitHub notifications/PRs, Linear tasks, optionally weather API.

**Example prompt:**
> Make me a daily briefing from my Google Calendar, GitHub notifications, and Linear tasks.

---

## Deep dive

**Description:** Turn a topic into a personalized audio explainer.

**Default input:** Cannot be fully defaulted — the topic is the user's intent; a deep dive on a topic they didn't choose is homework, not a gift. Suggest, don't pick.
**Input question:** "What should we dive into?" — present **3 personalized topic suggestions as options** (inferred from the user's repos, role, and recent activity — the same signals you'd have used to pick silently) plus the free-text field for their own topic. One click, no interview.

**One topic per episode.** Multiple sources are fine when they cover the same topic — never mix unrelated topics into one deep dive.

| Default | Value |
|---------|-------|
| Length | ~8 min |
| Segments | 4–6 (intro, background, key points, analysis, takeaways) |
| Show name | Deep Dives |
| Kokoro voice | af_heart |

**Data sources:** Web search.

**Example prompt:**
> Deep dive into how solar panels work

---

## Travel guide

**Description:** A private audio itinerary for your next trip.

**Default input:** Cannot be fully defaulted — destination is required.
**Input question:** "Where are you going and when?"

| Default | Value |
|---------|-------|
| Length | ~6 min |
| Segments | 4–5 (overview, getting around, must-see, food, tips) |
| Show name | Travel Guides |
| Kokoro voice | af_heart |

**Data sources:** Web search, travel APIs, location data.

**Example prompt:**
> I'm going to Lisbon next week for 4 days. Make me a travel guide.

---

## Meeting recap

**Description:** Drop in a transcript, get a summary with action items.

**Default input:** Cannot be fully defaulted — transcript is required.
**Input question:** "Paste or link the meeting transcript."

| Default | Value |
|---------|-------|
| Length | ~3 min |
| Segments | 3 (summary, decisions, action items) |
| Show name | Meeting Recaps |
| Kokoro voice | af_heart |

**Data sources:** User-provided transcript (paste, file, or URL).

**Example prompt:**
> Here's the transcript from today's standup: [paste]

---

## Sleep story

**Description:** A custom wind-down story set in your favorite place.

**Default input:** A slow, atmospheric story about a night train crossing quiet mountains in the rain.
**Input question (only if user asks to customize):** "What setting or mood would you like?"

| Default | Value |
|---------|-------|
| Length | ~5 min |
| Segments | 3–4 (setup, journey, resolution, goodnight) |
| Show name | Sleep Stories |
| Kokoro voice | af_nicole |

**Data sources:** User prompt (creative generation).

**Example prompt:**
> A sleep story about a lighthouse keeper on a calm autumn night.

---

## Language practice

**Description:** Spoken drills in the language you're learning, with pauses to answer out loud.

**Default input:** Cannot be fully defaulted — language is required.
**Input question:** "Which language are you learning, and roughly what level?"

| Default | Value |
|---------|-------|
| Length | ~10 min |
| Segments | 4–6 (warm-up vocab, phrases, listen-and-repeat, recall quiz, recap) |
| Show name | Language Practice |
| Kokoro voice | af_heart |

**Data sources:** User prompt (creative generation). Use the recall (3s) and speaking-practice (5s) pauses from audio-providers.md between prompts.

**Example prompt:**
> Spanish practice for a beginner — ordering food and asking directions.

---

## Sleep podcast

**Description:** A calm, droning deep-dive designed to put you to sleep.

**Default input:** The history and science of ocean currents.
**Input question (only if user asks to customize):** "What topic should I drone on about?"

| Default | Value |
|---------|-------|
| Length | ~20 min |
| Segments | 6–8 (slow, meandering, low-stakes) |
| Show name | Sleep Podcasts |
| Kokoro voice | af_nicole |

**Data sources:** Web search, Wikipedia.

**Example prompt:**
> A sleep podcast about the history of lighthouses.

---

## Using recipes

Recipes are pick-and-go. The user picks one and the agent starts generating immediately:

1. User picks a recipe (or the agent picks the best fit from their prompt)
2. Agent uses the **default input** — no questions asked
3. Smart defaults apply (language from locale, length from recipe, voice from TTS default)
4. Show auto-created from recipe name
5. Generate, preview, save

Only ask the input question if the recipe cannot be defaulted (travel guide needs a destination, meeting recap needs a transcript) or the user explicitly asks to customize.

After the first episode, users can customize any default through the standard interview flow in SKILL.md.
