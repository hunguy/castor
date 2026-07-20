from flask import Flask, request, jsonify
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urljoin, urlparse, urlunparse, unquote
from urllib.request import HTTPRedirectHandler, Request, build_opener
import base64
import subprocess, os


def _load_dotenv(path: Path) -> None:
    """Load KEY=VALUE lines into os.environ if the key is not already set."""
    if not path.is_file():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip().strip("'").strip('"')
        os.environ.setdefault(key, value)


_load_dotenv(Path(__file__).resolve().parent / ".env")

app = Flask(__name__)
TOKEN = os.environ["CASTOR_TOKEN"]
CONFIG_DIR = os.environ["CASTOR_CONFIG_DIR"]
CASTOR_BIN = os.environ.get("CASTOR_BIN", "castor")
BREW_BIN = os.path.dirname(CASTOR_BIN)

MEDIA_EXTS = (".mp4", ".mkv", ".avi", ".mov", ".webm", ".m4v", ".m3u8", ".mpd", ".ts")


class _Redirect(Exception):
    def __init__(self, url: str):
        self.url = url


class _StopRedirect(HTTPRedirectHandler):
    """Capture redirect Location without downloading the body."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise _Redirect(urljoin(req.full_url, headers["Location"]))


def cast_subcommand(target: str) -> str:
    """Use `cast url` for local files / direct media; `cast player` for pages."""
    if target.startswith("file://"):
        return "url"
    if os.path.isfile(target):
        return "url"
    path = unquote(urlparse(target).path).lower()
    if path.endswith(MEDIA_EXTS):
        return "url"
    return "player"


def with_basic_auth(target: str, username: str, password: str) -> str:
    """Embed HTTP Basic credentials in the URL for ffmpeg/castor to use."""
    parsed = urlparse(target)
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        return target
    userinfo = f"{quote(username, safe='')}:{quote(password, safe='')}"
    host = parsed.hostname
    if parsed.port:
        host = f"{host}:{parsed.port}"
    netloc = f"{userinfo}@{host}"
    return urlunparse(parsed._replace(netloc=netloc))


def resolve_direct_url(url: str, username: str = "", password: str = "", timeout: float = 30) -> str:
    """Follow one hop to a direct CDN URL (e.g. RD WebDAV → download.*).

    ffmpeg fails seeking MP4s through auth redirects (moov-at-end needs Range).
    The final Real-Debrid download URL supports Range without Basic auth.
    """
    req = Request(url, method="GET")
    # Avoid pulling the whole file if the server doesn't redirect.
    req.add_header("Range", "bytes=0-0")
    if username:
        token = base64.b64encode(f"{username}:{password}".encode()).decode()
        req.add_header("Authorization", f"Basic {token}")

    opener = build_opener(_StopRedirect)
    try:
        with opener.open(req, timeout=timeout) as resp:
            return resp.geturl()
    except _Redirect as e:
        return e.url


def redact_url(url: str) -> str:
    parsed = urlparse(url)
    host = parsed.hostname or ""
    if parsed.port:
        host = f"{host}:{parsed.port}"
    return urlunparse(parsed._replace(netloc=host))


@app.post("/cast")
def cast():
    env = os.environ.copy()
    env["PATH"] = f"{BREW_BIN}:{env.get('PATH', '')}"

    if request.headers.get("X-Auth") != TOKEN:
        return jsonify({"error": "unauthorized"}), 401
    data = request.get_json(silent=True) or {}
    target = data.get("path") or data.get("url")
    if not target:
        return jsonify({"error": "missing url"}), 400

    username = data.get("username") or os.environ.get("CASTOR_MEDIA_USER")
    password = data.get("password")
    if password is None:
        password = os.environ.get("CASTOR_MEDIA_PASS", "")

    resolved_via_redirect = False
    if username and urlparse(target).scheme in ("http", "https"):
        try:
            resolved = resolve_direct_url(target, username, password)
        except (HTTPError, URLError, TimeoutError, OSError) as e:
            return jsonify({"error": f"failed to resolve url: {e}"}), 502
        if urlparse(resolved).hostname != urlparse(target).hostname:
            # CDN / signed URL — no Basic auth needed (and auth breaks Range).
            target = resolved
            resolved_via_redirect = True
        else:
            target = with_basic_auth(resolved, username, password)

    cmd = cast_subcommand(target)
    subprocess.Popen([CASTOR_BIN, "cast", cmd, target], cwd=CONFIG_DIR, env=env)

    return jsonify({
        "status": "casting",
        "url": redact_url(target),
        "command": cmd,
        "auth": bool(username) and not resolved_via_redirect,
        "resolved": resolved_via_redirect,
    })


@app.get("/health")
def health():
    return jsonify({"ok": True})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8787)
