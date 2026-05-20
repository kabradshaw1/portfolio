"""JWT authentication dependency for FastAPI services."""

import os
from dataclasses import dataclass

import jwt
from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

_bearer_scheme = HTTPBearer(auto_error=False)
MIN_HS256_SECRET_BYTES = 32


@dataclass(frozen=True)
class AuthContext:
    subject: str
    email: str | None
    tier: str


def _csv_set(value: str | None) -> set[str]:
    return {part.strip().lower() for part in (value or "").split(",") if part.strip()}


def _tier_for(subject: str, email: str | None) -> str:
    operator_subjects = _csv_set(os.getenv("RAG_OPERATOR_USER_IDS"))
    operator_emails = _csv_set(os.getenv("RAG_OPERATOR_EMAILS"))
    if subject.lower() in operator_subjects:
        return "operator"
    if email and email.lower() in operator_emails:
        return "operator"
    return "user"


def _resolve_token(
    request: Request,
    credentials: HTTPAuthorizationCredentials | None,
) -> str | None:
    if credentials is not None:
        return credentials.credentials
    authorization = request.headers.get("authorization", "")
    scheme, _, token = authorization.partition(" ")
    if scheme.lower() == "bearer" and token:
        return token
    return request.cookies.get("access_token")


def create_auth_context_dependency(secret: str):
    """Create a dependency that returns subject, email, and authorization tier.

    When secret is empty, auth is disabled (all requests pass as anonymous).
    This allows compose-smoke CI tests to run without token generation.

    Auth resolution order:
    1. Authorization: Bearer <token> header (takes precedence)
    2. access_token cookie (set by Go auth service as httpOnly cookie)
    """
    if not secret:

        async def no_auth(
            request: Request | None = None,
            credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
        ) -> AuthContext:
            return AuthContext(subject="anonymous", email=None, tier="anonymous")

        return no_auth

    if len(secret.encode("utf-8")) < MIN_HS256_SECRET_BYTES:
        raise ValueError("JWT secret must be at least 32 bytes for HS256")

    async def require_auth(
        request: Request,
        credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    ) -> AuthContext:
        """Validate JWT from Bearer header or cookie and return auth context."""
        token = _resolve_token(request, credentials)
        if token is None:
            raise HTTPException(status_code=401, detail="Missing authorization")

        try:
            payload = jwt.decode(
                token,
                secret,
                algorithms=["HS256"],
                options={"require": ["sub", "exp"]},
            )
        except jwt.ExpiredSignatureError:
            raise HTTPException(status_code=401, detail="Token expired")
        except jwt.InvalidTokenError:
            raise HTTPException(status_code=401, detail="Invalid token")

        user_id = payload.get("sub")
        if not user_id:
            raise HTTPException(status_code=401, detail="Invalid token")
        email = payload.get("email")
        email = email if isinstance(email, str) and email else None
        return AuthContext(
            subject=str(user_id),
            email=email,
            tier=_tier_for(subject=str(user_id), email=email),
        )

    return require_auth


def create_auth_dependency(secret: str):
    """Create a FastAPI dependency that validates auth and returns userId."""
    auth_context = create_auth_context_dependency(secret)

    async def require_user_id(
        request: Request,
        credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    ) -> str:
        context = await auth_context(request, credentials)
        return context.subject

    return require_user_id
