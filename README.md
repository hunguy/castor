
<p align="center">
  <img src=".github/images/castor.svg" alt="Castor" width="200"/>
</p>

<p align="center">
  <a href="https://github.com/stupside/castor/releases/latest">
    <img src="https://img.shields.io/github/v/release/stupside/castor?style=flat-square" alt="Latest Release">
  </a>
  <a href="https://pkg.go.dev/github.com/stupside/castor">
    <img src="https://img.shields.io/badge/Go-Reference-00ADD8?style=flat-square&logo=go" alt="Go Reference">
  </a>
  <a href="https://github.com/stupside/homebrew-tap/blob/main/Casks/castor.rb">
    <img src="https://img.shields.io/badge/Homebrew-Available-FBB040?style=flat-square&logo=homebrew" alt="Homebrew">
  </a>
  <a href="https://github.com/stupside/castor/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/stupside/castor?style=flat-square" alt="License">
  </a>
  <a href="https://github.com/stupside/castor/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/stupside/castor/continuous-integration.yml?style=flat-square" alt="Build Status">
  </a>
</p>

# Castor

Smart TVs won't cast arbitrary web video, and screen mirroring is laggy and drops resolution. Castor casts the real stream instead, at full quality, from your terminal.

*I built it because I couldn't cast web video from my laptop to my TV: no Chromecast, no AirPlay.*

Point it at any web page and Castor finds the video, extracts the stream, transcodes it for your TV, and casts in real time. It also takes a direct stream URL or an IMDB/TMDB id, and can burn in auto-generated subtitles.

<p align="center">
  <img src=".github/images/screen-selection.png" alt="Browsing titles in the castor TUI" width="640"/>
  <br/>
  <sub><em>Run <code>castor cast</code> to browse and search titles, inspect posters and metadata, then cast, without leaving the terminal.</em></sub>
</p>

> [!NOTE]
> **How extraction works**
>
> Castor launches headless Chrome with a randomized fingerprint and stealth scripts to hide automation. It watches all network traffic over the Chrome DevTools Protocol to capture the video stream, then runs a short action pipeline: click the page, navigate into the largest iframe, solve a Cloudflare Turnstile if one appears, and click again as a fallback.
>
> This works on most streaming sites but won't beat sophisticated bot protection.


## Installation

The recommended way to run Castor is the **native binary**. It runs directly on your machine, so it shares your TV's network, which device discovery needs. It requires **Chrome/Chromium** (headless extraction), **ffmpeg** (transcoding), and **ffprobe** (format detection) on your `PATH`. [Docker](#docker-optional) is an optional alternative that bundles all three, but only works from a Linux host.

### Homebrew (macOS)

```sh
brew install --cask stupside/tap/castor
```

See [Quick start](#quick-start) to create the one-time `config.yaml` (which TV, which sources). After that, casting is a single command, no URL, just an IMDB/TMDB id:

```sh
castor cast movie tt12300742
```

### From source

Needs Go 1.26+ and cmake (the whisper.cpp bindings are cgo and link a locally built `libwhisper.a`):

```sh
git clone --recurse-submodules https://github.com/stupside/castor.git
cd castor
make          # builds libwhisper.a, then the castor binary
```

`go install` won't work: the vendored whisper.cpp bindings come in through a local `replace` and need that prebuilt static lib.

### Docker (optional)

> [!WARNING]
> **Docker can only reach your TV from a Linux host on the same LAN.** Discovery is SSDP multicast and the TV streams back from Castor's replay server, and neither survives Docker's bridge network, so `--network host` is required. But on **Docker Desktop (macOS/Windows)** `--network host` is a silent no-op: the container lands on Docker Desktop's internal VM subnet (e.g. `192.168.65.x`), never your real LAN, so `scan` finds nothing and `cast` fails with `device "…" (type dlna) not found` even though the TV is up. **No `docker run` flag fixes this.** On macOS/Windows, run the [native binary](#homebrew-macos) instead. Or point Docker at a Linux VM bridged onto your LAN (e.g. Lima + `socket_vmnet`), which is the only way a container gets a real address on your network.

On a Linux box or NAS on the same network as the TV, the prebuilt `ghcr.io/stupside/castor` image bundles Chrome, ffmpeg and ffprobe so you don't install them by hand:

```sh
# Discover devices (no config required)
docker run --rm --network host ghcr.io/stupside/castor:latest scan

# Cast a movie by id, mounting config.yaml and a persistent model cache
docker run --rm --network host \
  -v "$PWD/config.yaml:/config.yaml" \
  -v castor-cache:/root/.cache \
  ghcr.io/stupside/castor:latest \
  cast movie tt12300742
```

The `-v "$PWD/config.yaml:/config.yaml"` mount is what makes this work: the container reads your device and sources from [`config.yaml`](config.yaml) at `/config.yaml`, so run every command from the directory holding it. The `castor-cache` volume keeps the auto-downloaded whisper models (~75 MB) between runs; swap `:latest` for any release tag to pin a version.


## Supported devices

### DLNA / UPnP

Any TV implementing the DLNA/UPnP `MediaRenderer:1` profile works, which covers virtually every smart TV sold in the last decade: **Samsung** (tested), **LG**, **Sony Bravia**, **Panasonic Viera**, **Philips**, **Hisense**, **TCL**, **VIZIO**, **Sharp**. Networked players like Kodi, VLC, and Plex also work.

Run `castor scan` to discover devices on your network.

### Chromecast

> [!WARNING]
> Experimental: implemented but untested. Contributions welcome.


## Quick start

Castor **requires a `config.yaml`** in the current directory (or pass `--config`). Everything mechanical ships with working defaults, so a minimal file only has to say **which device to cast to** and **which sources to cast from**. A [TMDB API key](https://www.themoviedb.org/settings/api) is optional, needed only for the interactive browser.

```sh
# 1. Find your TV's exact name
castor scan
```

Create `config.yaml` with that name:

```yaml
device:
  name: "Living Room TV"   # exact name from `castor scan`
  type: dlna

sources:
  - proxies: ["https://vidsrc-embed.ru"]
    templates:
      movie: "/embed/movie/{itemID}"
      episode: "/embed/tv/{itemID}/{season}-{episode}"

# tmdb:
#   api_key: "<KEY>"   # optional, only for the `castor cast` browser; free from https://www.themoviedb.org/settings/api
```

That's all you need to cast by id, the quickest path with no TMDB key:

```sh
# 2. Cast a movie straight from an IMDB/TMDB id, resolved through your sources
castor cast movie tt12300742
```

> [!NOTE]
> **Sources can change.** `cast movie` resolves the id against the `proxies` you set in [`config.yaml`](config.yaml). These are external sites, so one can go offline or move without notice. If a cast stops resolving, update that entry in the `proxies` list or add another.

Prefer to browse? Add a `tmdb.api_key` and run `castor cast` for an interactive TUI. It first asks which device to cast to: every DLNA/UPnP renderer on your network, discovered on the fly and with your configured device pre-selected:

<p align="center">
  <img src=".github/images/screen-devices.png" alt="Selecting a cast target in the castor TUI" width="640"/>
</p>

Then it opens a TMDB-backed browser: filter by genre, search, inspect posters and metadata, drill into a series' episodes, and cast the one you pick.


## Usage

```sh
# Interactive TMDB browser: search, pick a movie/episode, cast (needs tmdb.api_key)
castor cast

# Cast whatever video is playing on a web page
castor cast player https://example.com/watch/some-video

# Cast by IMDB/TMDB id, using the sources in your config
castor cast movie   tt33028778
castor cast episode tt2699128 --season 1 --episode 3

# Cast a raw stream URL directly
castor cast url https://example.com/stream.m3u8

# Useful flags
castor cast movie --dry-run tt33028778   # print found URLs without casting
castor --debug cast player https://...   # verbose logging
castor scan                              # discover devices on the network
castor info                              # version / build info
```


## Configuration

[Quick start](#quick-start) covers the required keys. Beyond those, everything mechanical (timeouts, probing, capture, transcoding, Chrome discovery) ships with working defaults. Override any of it in `config.yaml`, point at a different file with `--config`, drop secrets like your TMDB key into a git-ignored sibling `config.local.yaml` (it overlays `config.yaml`), or set `CASTOR_SECTION__FIELD` environment variables.

The one opt-in worth calling out is auto-generated subtitles, burned into the video:

```yaml
whisper:
  enable: true             # off by default
  # language: "fr"         # default: English
  # model_path: ""         # default: ggml-tiny.en (~75 MB, auto-downloaded)
```


## Disclaimer

Castor hosts no video and ships no content of its own. It's a general tool for casting a stream to your TV, not tied to any particular website. The sources in the example `config.yaml` are just that, examples; which sites you point it at, and staying within the law and their terms of use, is your responsibility. Only cast content you have the right to access.


## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
