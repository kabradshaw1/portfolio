"""Tests for shared.auth JWT authentication dependency."""

import time

import anyio
import jwt
from fastapi import Depends, FastAPI
from starlette.requests import Request
from starlette.testclient import TestClient

from shared.auth import create_auth_context_dependency, create_auth_dependency

SECRET = "test-secret-key-at-least-32-bytes"
ALGORITHM = "HS256"


def _make_token(secret: str, sub: str = "user-123", exp_offset: int = 3600) -> str:
    """Helper to create a signed JWT token."""
    payload = {
        "sub": sub,
        "email": "test@example.com",
        "exp": int(time.time()) + exp_offset,
    }
    return jwt.encode(payload, secret, algorithm=ALGORITHM)


def _token(
    secret: str,
    sub: str = "user-123",
    email: str | None = "test@example.com",
    exp_offset: int = 3600,
) -> str:
    payload = {
        "sub": sub,
        "exp": int(time.time()) + exp_offset,
    }
    if email is not None:
        payload["email"] = email
    return jwt.encode(payload, secret, algorithm=ALGORITHM)


def _request(headers: dict[str, str] | None = None) -> Request:
    raw_headers = [
        (key.lower().encode("latin-1"), value.encode("latin-1"))
        for key, value in (headers or {}).items()
    ]
    return Request(
        {
            "type": "http",
            "method": "GET",
            "path": "/protected",
            "headers": raw_headers,
        }
    )


def _make_app(secret: str) -> FastAPI:
    """Create a minimal FastAPI app using the auth dependency."""
    app = FastAPI()
    auth_dep = create_auth_dependency(secret)

    @app.get("/protected")
    async def protected(user_id: str = Depends(auth_dep)):
        return {"user_id": user_id}

    return app


# ---------------------------------------------------------------------------
# Empty secret — anonymous access
# ---------------------------------------------------------------------------


def test_empty_secret_allows_anonymous():
    """When secret is empty, all requests pass through as 'anonymous'."""
    client = TestClient(_make_app(""))
    resp = client.get("/protected")
    assert resp.status_code == 200
    assert resp.json() == {"user_id": "anonymous"}


def test_short_secret_rejected_when_auth_enabled():
    """HS256 auth must not run with weak HMAC secrets."""
    try:
        create_auth_context_dependency("short-secret")
    except ValueError as exc:
        assert "at least 32 bytes" in str(exc)
    else:
        raise AssertionError("short JWT secret should be rejected")


# ---------------------------------------------------------------------------
# Bearer header — happy path
# ---------------------------------------------------------------------------


def test_valid_bearer_returns_user_id():
    """Valid Bearer token returns the sub claim as user_id."""
    token = _make_token(SECRET, sub="user-abc")
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200
    assert resp.json() == {"user_id": "user-abc"}


# ---------------------------------------------------------------------------
# Bearer header — error paths
# ---------------------------------------------------------------------------


def test_expired_bearer_returns_401():
    """Expired Bearer token returns 401."""
    token = _make_token(SECRET, exp_offset=-10)
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 401


def test_wrong_secret_bearer_returns_401():
    """Bearer token signed with wrong secret returns 401."""
    token = _make_token("wrong-secret")
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 401


# ---------------------------------------------------------------------------
# Cookie — happy path
# ---------------------------------------------------------------------------


def test_valid_cookie_returns_user_id():
    """Valid access_token cookie returns the sub claim as user_id."""
    token = _make_token(SECRET, sub="cookie-user")
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected", cookies={"access_token": token})
    assert resp.status_code == 200
    assert resp.json() == {"user_id": "cookie-user"}


# ---------------------------------------------------------------------------
# Cookie — error paths
# ---------------------------------------------------------------------------


def test_expired_cookie_returns_401():
    """Expired access_token cookie returns 401."""
    token = _make_token(SECRET, exp_offset=-10)
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected", cookies={"access_token": token})
    assert resp.status_code == 401


# ---------------------------------------------------------------------------
# No auth at all
# ---------------------------------------------------------------------------


def test_no_auth_returns_401():
    """Request with no Bearer header and no cookie returns 401."""
    client = TestClient(_make_app(SECRET))
    resp = client.get("/protected")
    assert resp.status_code == 401


# ---------------------------------------------------------------------------
# Precedence: Bearer header takes priority over cookie
# ---------------------------------------------------------------------------


def test_bearer_takes_precedence_over_cookie():
    """When both Bearer and cookie are present, Bearer wins."""
    bearer_token = _make_token(SECRET, sub="bearer-user")
    cookie_token = _make_token(SECRET, sub="cookie-user")
    client = TestClient(_make_app(SECRET))
    resp = client.get(
        "/protected",
        headers={"Authorization": f"Bearer {bearer_token}"},
        cookies={"access_token": cookie_token},
    )
    assert resp.status_code == 200
    assert resp.json() == {"user_id": "bearer-user"}


def test_auth_context_classifies_operator_by_email(monkeypatch):
    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "kyle@example.test")
    dependency = create_auth_context_dependency(SECRET)
    token = _token(SECRET, sub="user-1", email="kyle@example.test")
    request = _request(headers={"Authorization": f"Bearer {token}"})

    context = anyio.run(dependency, request, None)

    assert context.subject == "user-1"
    assert context.email == "kyle@example.test"
    assert context.tier == "operator"


def test_auth_context_classifies_normal_user(monkeypatch):
    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "kyle@example.test")
    dependency = create_auth_context_dependency(SECRET)
    token = _token(SECRET, sub="user-2", email="other@example.test")
    request = _request(headers={"Authorization": f"Bearer {token}"})

    context = anyio.run(dependency, request, None)

    assert context.subject == "user-2"
    assert context.email == "other@example.test"
    assert context.tier == "user"


def test_auth_context_preserves_anonymous_when_secret_empty():
    dependency = create_auth_context_dependency("")
    request = _request()

    context = anyio.run(dependency, request, None)

    assert context.subject == "anonymous"
    assert context.email is None
    assert context.tier == "anonymous"


def test_auth_context_rejects_malformed_token():
    dependency = create_auth_context_dependency(SECRET)
    request = _request(headers={"Authorization": "Bearer not-a-jwt"})

    try:
        anyio.run(dependency, request, None)
    except Exception as exc:
        assert getattr(exc, "status_code") == 401
    else:
        raise AssertionError("malformed token should be rejected")
