#!/usr/bin/env python3
"""Real-time verification console for the smart-model-router CLIProxyAPI plugin.

Auth rules:
- Endpoints under /v0/... (management) use API_KEY.
- Endpoints under /v1/... (models, chat) use API_KEY_MODELS.

Both are sent as: Authorization: Bearer <token>
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

DEFAULT_BASE_URL = "http://localhost:8317"
DEFAULT_VIRTUAL_MODEL = "auto-model"
DEFAULT_PROMPT = "Summarize in one sentence whether the smart-model-router plugin is working."
PLUGIN_ID = "smart-model-router"
ENV_FILE = Path(__file__).resolve().parents[1] / ".env"

# ANSI colors (disabled automatically when stdout is not a TTY).
USE_COLOR = sys.stdout.isatty()


def c(code: str, text: str) -> str:
    if not USE_COLOR:
        return text
    return f"\033[{code}m{text}\033[0m"


def bold(text: str) -> str:
    return c("1", text)


def green(text: str) -> str:
    return c("32", text)


def red(text: str) -> str:
    return c("31", text)


def yellow(text: str) -> str:
    return c("33", text)


def cyan(text: str) -> str:
    return c("36", text)


def dim(text: str) -> str:
    return c("2", text)


def ok(flag: bool) -> str:
    return green("OK") if flag else red("FAIL")


class HttpError(RuntimeError):
    pass


def load_dotenv(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def normalize_base_url(url: str) -> str:
    return url.strip().rstrip("/")


def request_json(
    method: str, url: str, token: str, payload: dict | None = None
) -> object:
    headers = {
        "Accept": "application/json, text/plain, */*",
        "Authorization": f"Bearer {token}",
    }
    body = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        body = json.dumps(payload).encode("utf-8")

    try:
        with urlopen(
            Request(url, data=body, headers=headers, method=method), timeout=30
        ) as resp:
            text = resp.read().decode("utf-8", errors="replace")
    except HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise HttpError(f"HTTP {exc.code} {exc.reason} at {url}: {detail}") from exc
    except URLError as exc:
        raise HttpError(f"Network error at {url}: {exc.reason}") from exc

    if not text.strip():
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return text


def safe_request(
    method: str, url: str, token: str, payload: dict | None = None
) -> tuple[object, str | None]:
    try:
        return request_json(method, url, token, payload), None
    except HttpError as exc:
        return None, str(exc)


def section(title: str) -> None:
    print(f"\n{bold(cyan('== ' + title + ' =='))}")


def kv(label: str, value: object, width: int = 22) -> None:
    print(f"  {dim(label.ljust(width))} {value}")


def pretty_json(value: object) -> None:
    print(json.dumps(value, indent=2, ensure_ascii=False, sort_keys=True))


def render_plugins(data: object, error: str | None) -> None:
    section("Plugins (/v0/management/plugins) - API_KEY")
    if error:
        kv("status", red("ERROR"))
        kv("detail", error)
        return

    if not isinstance(data, dict):
        pretty_json(data)
        return

    plugin = next(
        (p for p in data.get("plugins", []) if p.get("id") == PLUGIN_ID), None
    )
    kv("plugins_enabled", ok(bool(data.get("plugins_enabled"))))
    if not plugin:
        kv("plugin_found", red("NOT FOUND"))
        return

    kv("plugin_id", plugin.get("id"))
    kv("configured", ok(bool(plugin.get("configured"))))
    kv("registered", ok(bool(plugin.get("registered"))))
    kv("enabled", ok(bool(plugin.get("enabled"))))
    kv("effective_enabled", ok(bool(plugin.get("effective_enabled"))))
    kv("version", (plugin.get("metadata") or {}).get("version"))


def render_models(data: object, error: str | None, virtual_model: str) -> None:
    section("Models (/v1/models) - API_KEY_MODELS")
    if error:
        kv("status", red("ERROR"))
        kv("detail", error)
        return

    items = data.get("data", []) if isinstance(data, dict) else []
    ids = [m.get("id") for m in items if isinstance(m, dict)]
    found = virtual_model in ids
    kv("total_models", len(ids))
    kv("virtual_model", virtual_model)
    kv("virtual_model_found", ok(found))


def render_status(
    data: object, error: str | None, title: str, show_cache: bool = False
) -> None:
    section(f"{title} (/v0/management/plugins/{PLUGIN_ID}/status) - API_KEY")
    if error:
        kv("status", red("ERROR"))
        kv("detail", error)
        return

    if not isinstance(data, dict):
        pretty_json(data)
        return

    kv("plugin", data.get("plugin"))
    kv("virtual_model", data.get("virtual_model"))
    kv("strategy", data.get("strategy"))

    usage = data.get("usage") or {}
    kv("requests", usage.get("requests"))
    kv("failures", usage.get("failures"))

    state = data.get("state") or {}
    counters = state.get("counters") or {}
    kv("router_requests_total", counters.get("router_requests_total"))
    kv("router_cache_hits", counters.get("router_cache_hits"))
    kv("router_classifier_calls", counters.get("router_classifier_calls"))

    per_model_usage = state.get("usage") or {}
    if per_model_usage:
        print(f"\n  {bold('per-model usage:')}")
        for model_id, m in sorted(per_model_usage.items()):
            print(
                f"    {yellow(model_id):40s} "
                f"requests={m.get('requests', 0):<6} "
                f"failures={m.get('failures', 0):<4} "
                f"tokens={m.get('total_tokens', 0)}"
            )

    route_cache = state.get("route_cache") or {}
    if show_cache and route_cache:
        print(f"\n  {bold('route cache entries:')} {len(route_cache)}")


def render_chat(data: object, error: str | None) -> None:
    section("Chat completion test (/v1/chat/completions) - API_KEY_MODELS")
    if error:
        kv("status", red("ERROR"))
        kv("detail", error)
        return

    if not isinstance(data, dict):
        pretty_json(data)
        return

    model = data.get("model")
    usage = data.get("usage") or {}
    choices = data.get("choices") or []
    content = ""
    if choices and isinstance(choices[0], dict):
        content = (choices[0].get("message") or {}).get("content", "")

    kv("resolved_model", model)
    kv("prompt_tokens", usage.get("prompt_tokens"))
    kv("completion_tokens", usage.get("completion_tokens"))
    kv("total_tokens", usage.get("total_tokens"))
    print(f"\n  {bold('response:')}")
    print(f"  {content}")


def check_once(
    base_url: str,
    management_token: str,
    models_token: str,
    virtual_model: str,
    prompt: str,
    all_tests: bool,
    verbose: bool,
) -> None:
    plugins, plugins_err = safe_request(
        "GET", f"{base_url}/v0/management/plugins", management_token
    )
    models, models_err = safe_request("GET", f"{base_url}/v1/models", models_token)
    status, status_err = safe_request(
        "GET", f"{base_url}/v0/management/plugins/{PLUGIN_ID}/status", management_token
    )

    render_plugins(plugins, plugins_err)
    render_models(models, models_err, virtual_model)
    render_status(status, status_err, "Router status", show_cache=all_tests)

    if verbose:
        section("raw plugins")
        pretty_json(plugins)
        section("raw models")
        pretty_json(models)
        section("raw status")
        pretty_json(status)

    if not all_tests:
        return

    completion, chat_err = safe_request(
        "POST",
        f"{base_url}/v1/chat/completions",
        models_token,
        {
            "model": virtual_model,
            "messages": [{"role": "user", "content": prompt}],
            "stream": False,
        },
    )
    render_chat(completion, chat_err)

    status_after, status_after_err = safe_request(
        "GET", f"{base_url}/v0/management/plugins/{PLUGIN_ID}/status", management_token
    )
    render_status(
        status_after, status_after_err, "Router status after chat", show_cache=True
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Verify smart-model-router through CLIProxyAPI."
    )
    parser.add_argument("--base-url", help="Overrides BASE_URL from .env.")
    parser.add_argument("--virtual-model", default=DEFAULT_VIRTUAL_MODEL)
    parser.add_argument("--prompt", default=DEFAULT_PROMPT)
    parser.add_argument("--interval", type=int, default=5)
    parser.add_argument("--once", action="store_true")
    parser.add_argument(
        "--all",
        action="store_true",
        help="Run chat test and show cache/status after chat.",
    )
    parser.add_argument(
        "--verbose", action="store_true", help="Also print raw JSON responses."
    )
    args = parser.parse_args()

    env = load_dotenv(ENV_FILE)
    management_token = (
        env.get("API_KEY", "").strip() or os.getenv("API_KEY", "").strip()
    )
    models_token = (
        env.get("API_KEY_MODELS", "").strip() or os.getenv("API_KEY_MODELS", "").strip()
    )
    base_url = normalize_base_url(
        args.base_url
        or env.get("BASE_URL")
        or os.getenv("BASE_URL")
        or DEFAULT_BASE_URL
    )

    if not management_token:
        print(red("Missing management token. Add API_KEY=... to .env"), file=sys.stderr)
        return 2
    if not models_token:
        print(
            red("Missing models token. Add API_KEY_MODELS=... to .env"), file=sys.stderr
        )
        return 2

    print(bold(f"smart-model-router live check"))
    kv("env_file", ENV_FILE)
    kv("base_url", base_url)
    kv("management_auth", f"Bearer API_KEY ({len(management_token)} chars)")
    kv("models_auth", f"Bearer API_KEY_MODELS ({len(models_token)} chars)")
    kv("virtual_model", args.virtual_model)
    kv("mode", "all tests" if args.all else "basic checks")
    kv("interval", f"{args.interval}s" if not args.once else "single run")

    while True:
        print(bold(f"\n{'-' * 70}"))
        print(bold(time.strftime("[%Y-%m-%d %H:%M:%S]")))
        try:
            check_once(
                base_url,
                management_token,
                models_token,
                args.virtual_model,
                args.prompt,
                args.all,
                args.verbose,
            )
        except HttpError as exc:
            print(red(f"ERROR: {exc}"), file=sys.stderr)
        if args.once:
            return 0
        time.sleep(max(1, args.interval))


if __name__ == "__main__":
    raise SystemExit(main())
