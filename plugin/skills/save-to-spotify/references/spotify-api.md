# Spotify Web API (catalog lookups)

Use this reference when a segment names something that already exists on Spotify and the timeline should include a `spotify_entity` companion. This file covers the `save-to-spotify`-specific wiring: how to get a bearer token from the CLI, how to resolve names to `spotify:...` URIs, and when to omit a low-confidence entity.

Treat Spotify for Developers as the source of truth for endpoint shapes, parameters, schemas, and policy. Use the Spotify Web API rather than third-party catalogs, because timeline entries need Spotify URIs rather than cross-platform tap-throughs.

## Start with Spotify for Developers

Before scripting Web API calls, load the official developer context:

- `https://developer.spotify.com/llms.txt` - LLM-ready entrypoint for Spotify's developer platform.
- `https://developer.spotify.com/documentation/web-api/tutorials/building-with-ai` - Web API guidance for AI coding assistants and agents.
- `https://developer.spotify.com/reference/web-api/open-api-schema.yaml` - full OpenAPI 3.0 schema for the Spotify Web API.

The OpenAPI schema declares the base server (`https://api.spotify.com/v1`), paths, parameters, response schemas, and OAuth requirements. If the official docs or schema disagree with this local reference, follow `developer.spotify.com`.

Quick spec workflow:

```shell
SPEC_URL="https://developer.spotify.com/reference/web-api/open-api-schema.yaml"
curl -fsSL "$SPEC_URL" -o spotify-openapi.yaml
rg -n "^  /search:|operationId: search|get-an-album|get-an-artists-albums" spotify-openapi.yaml
```

## Getting a bearer token

`save-to-spotify token` prints the current access token to stdout. That token has been verified to work as a Spotify Web API `Bearer` token for requests against `api.spotify.com`, so agents do not need a separate app registration for the catalog lookups below.

Guard the call because it exits non-zero when the stored token cannot be refreshed:

```shell
if ! TOKEN=$(save-to-spotify token); then
  echo "not authenticated; run: save-to-spotify auth login" >&2
  exit 1
fi
```

For a quick auth smoke test against the catalog API:

```shell
curl -sfG "https://api.spotify.com/v1/search" \
  -H "Authorization: Bearer $TOKEN" \
  --data-urlencode "q=artist:The Beatles" \
  --data-urlencode "type=artist" \
  --data-urlencode "limit=1" >/dev/null
```

No extra scope grant is needed for public catalog search and metadata lookups. Check the OpenAPI `security` entries before using user-private data or mutating endpoints.

## Resolving names to Spotify URIs

Pattern: `GET /v1/search?q=<query>&type=<entity>&limit=5`, then pick the best match from `.<entity>s.items`.

For albums and tracks, use field-qualified queries (`artist:X album:Y` or `artist:X track:Y`) for higher precision than free text:

```shell
TOKEN=$(save-to-spotify token) || exit 1
curl -sG "https://api.spotify.com/v1/search" \
  -H "Authorization: Bearer $TOKEN" \
  --data-urlencode "q=artist:The Beatles album:Abbey Road" \
  --data-urlencode "type=album" \
  --data-urlencode "limit=5" \
| jq '.albums.items[] | {uri, name, release_date, artists: [.artists[].name]}'
```

Minimal dependency-free Python wrapper:

```python
import json
import subprocess
from urllib.parse import urlencode
from urllib.request import Request, urlopen

API = "https://api.spotify.com/v1"

def spotify_token():
    return subprocess.run(
        ["save-to-spotify", "token"],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()

def spotify_get(path, params):
    url = f"{API}{path}?{urlencode(params)}"
    req = Request(url, headers={"Authorization": f"Bearer {spotify_token()}"})
    with urlopen(req, timeout=20) as resp:
        return json.load(resp)

def spotify_search(query, entity_type, limit=5):
    data = spotify_get("/search", {"q": query, "type": entity_type, "limit": limit})
    return data.get(f"{entity_type}s", {}).get("items", [])
```

For ambiguous names (self-titled albums, common words, cover-versus-original tracks), score each result before accepting it. Weight artist-name overlap, title overlap, and entity-specific metadata like `album_type`, `release_date`, `show.name`, or track duration. Drop the `spotify_entity` companion rather than ship a wrong URI when no result scores confidently.

### Entity query patterns

Albums and tracks support the field-qualified form above. Artists, playlists, shows, and episodes take free-text queries; feed the user-visible name and change the `type=` parameter:

| Entity   | Query                      | `type=`    | Response path        | Output URI             |
|----------|----------------------------|------------|----------------------|------------------------|
| Album    | `artist:X album:Y`         | `album`    | `.albums.items[]`    | `spotify:album:...`    |
| Track    | `artist:X track:Y`         | `track`    | `.tracks.items[]`    | `spotify:track:...`    |
| Artist   | `X` (name)                 | `artist`   | `.artists.items[]`   | `spotify:artist:...`   |
| Playlist | `X` (name)                 | `playlist` | `.playlists.items[]` | `spotify:playlist:...` |
| Show     | `X` (name)                 | `show`     | `.shows.items[]`     | `spotify:show:...`     |
| Episode  | `show title episode title` | `episode`  | `.episodes.items[]`  | `spotify:episode:...`  |

Podcast episode results are noisier than track and album search; verify the top hit's `show.name` field before accepting.

## Useful catalog endpoints

Use the OpenAPI schema before expanding this list, but these are the common calls for timeline entity resolution:

| Need                                      | Endpoint                                                                          |
|-------------------------------------------|-----------------------------------------------------------------------------------|
| Search any catalog type                   | `GET /v1/search?q=...&type=album,track,artist,playlist,show,episode`              |
| Full album / track / artist metadata      | `GET /v1/albums/{id}`, `GET /v1/tracks/{id}`, or `GET /v1/artists/{id}`           |
| Artist's recent albums, excluding singles | `GET /v1/artists/{id}/albums?include_groups=album&limit=50`                       |
| Playlist metadata and tracks              | `GET /v1/playlists/{id}`                                                          |
| Show / episode metadata                   | `GET /v1/shows/{id}` or `GET /v1/episodes/{id}` with `market=<ISO country>`       |
| Show's episode list                       | `GET /v1/shows/{id}/episodes?market=<ISO country>&limit=50`                       |
| New Releases, editorial albums only       | `GET /v1/browse/new-releases?country=US&limit=50`                                 |

For endpoints with a `market` parameter, use an ISO 3166-1 alpha-2 country code when needed. With a valid user access token, the country associated with the user account takes priority over the query parameter; see the OpenAPI `QueryMarket` parameter before adding market-specific logic.

## Fallbacks and non-Spotify data

When sourcing a Spotify-native reference, the fallback hierarchy is:

1. Spotify Web API - primary. Always try it first when auth is valid.
2. No entity - omit the `spotify_entity` companion rather than invent or guess a URI.

If Spotify auth fails, fix auth first with `save-to-spotify auth login`. Do not substitute for other third-party catalogs as the timeline destination just because auth failed. Those sources can help verify dates, credits, or artwork, but they do not produce the `spotify:...` URI the Now Playing View needs.

## Rate limits

Spotify's Web API is rate-limited per token; a sensible guardrail is about 10 req/s with small jitter. On HTTP 429, honor the `Retry-After` header. A typical episode's 10-20 lookups are well under the limit.
