"""ASGI Host header validation for Starlette-based services."""

from starlette.types import ASGIApp, Receive, Scope, Send

INVALID_HOST_CHARS = frozenset("/?#\\")


class HostHeaderValidationMiddleware:
    """Reject malformed Host headers before Starlette constructs request.url."""

    def __init__(self, app: ASGIApp):
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        host_headers = [
            value for name, value in scope.get("headers", []) if name.lower() == b"host"
        ]
        if any(_is_malformed_host(value) for value in host_headers):
            await _send_bad_request(send)
            return

        await self.app(scope, receive, send)


def _is_malformed_host(raw_host: bytes) -> bool:
    try:
        host = raw_host.decode("ascii")
    except UnicodeDecodeError:
        return True
    return any(char in INVALID_HOST_CHARS or char.isspace() for char in host)


async def _send_bad_request(send: Send) -> None:
    body = b'{"detail":"Invalid Host header"}'
    await send(
        {
            "type": "http.response.start",
            "status": 400,
            "headers": [
                (b"content-type", b"application/json"),
                (b"content-length", str(len(body)).encode("ascii")),
            ],
        }
    )
    await send({"type": "http.response.body", "body": body})
