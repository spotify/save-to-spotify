# CI / Automation

How to use save-to-spotify in GitHub Actions, cron jobs, and other non-interactive environments.

## Headless authentication

Use `--no-browser` for remote or CI environments where a browser is not available. The CLI prints a URL that a **human** must open in their browser to authorize, then paste back the redirect URL.

```bash
save-to-spotify auth login --no-browser
```

This is a one-time step. After initial auth, the CLI stores credentials locally and refreshes the access token automatically on every subsequent run.

## Persisting credentials for scheduled workflows

The CLI stores two files after `auth login`:

- `~/.config/save-to-spotify/token.json` — access token + refresh token
- `~/.config/save-to-spotify/dpop_key.json` — DPoP key pair (required for token refresh)

The refresh token is long-lived. The CLI automatically refreshes the short-lived access token before each API call — no manual rotation needed.

> **Avoid `SAVE_TO_SPOTIFY_AUTH_TOKEN` for scheduled automation.**
> That env var injects a raw access token that expires in ~1 hour and bypasses the CLI's auto-refresh mechanism. It works for one-off scripting, but will silently fail in any workflow that runs on a schedule.

## Setup (one-time)

1. Authenticate locally:

   ```bash
   save-to-spotify auth login
   ```

2. Encode the credential files as base64 (the `-w 0` flag avoids line wrapping):

   ```bash
   base64 -w 0 < ~/.config/save-to-spotify/token.json
   base64 -w 0 < ~/.config/save-to-spotify/dpop_key.json
   ```

3. Store the two base64 strings as CI secrets (e.g. `STS_TOKEN_B64` and `STS_DPOP_KEY_B64` in GitHub → Settings → Secrets and variables → Actions).

## GitHub Actions

```yaml
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - name: Install save-to-spotify
        run: curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --no-skills

      - name: Restore credentials
        run: |
          mkdir -p ~/.config/save-to-spotify
          printf '%s' "${{ secrets.STS_TOKEN_B64 }}" | base64 -d > ~/.config/save-to-spotify/token.json
          printf '%s' "${{ secrets.STS_DPOP_KEY_B64 }}" | base64 -d > ~/.config/save-to-spotify/dpop_key.json
          chmod 600 ~/.config/save-to-spotify/token.json ~/.config/save-to-spotify/dpop_key.json

      - name: Verify auth
        run: save-to-spotify auth status

      - name: Upload episode
        run: save-to-spotify upload ./episode.mp3 --title "Daily Briefing"
```

The CLI refreshes the access token automatically on every run. This setup does not require periodic secret rotation.

## Other CI systems

The same pattern applies to any CI provider: write `token.json` and `dpop_key.json` to `~/.config/save-to-spotify/` (or `$XDG_CONFIG_HOME/save-to-spotify/`) from your CI secrets before invoking the CLI. Both files are required for DPoP-bound tokens (the default).

## Health checks

Use `auth status` as a pre-flight check in CI — it exits non-zero when the token is expired:

```bash
save-to-spotify auth status          # human-readable
save-to-spotify --json auth status   # structured output for scripting
```

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `401` after ~1 hour | Using `SAVE_TO_SPOTIFY_AUTH_TOKEN` env var | Unset the env var; use file-based auth instead |
| `invalid_request` on token refresh | Missing `dpop_key.json` | Persist both files, not just `token.json` |
| `DPoP key is corrupt` | Key file was truncated or modified | Re-run `auth login` locally and re-export the secrets |
| `not authenticated` | Files not restored to the correct path | Check `$XDG_CONFIG_HOME` or use the default `~/.config/save-to-spotify/` |
| `token refresh failed` | Refresh token revoked or expired | Re-run `auth login` locally and update CI secrets |
