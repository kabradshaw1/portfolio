"""JWT authentication dependency for FastAPI services."""

import jwt
from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

_bearer_scheme = HTTPBearer(auto_error=False)


def create_auth_dependency(secret: str):
    """Create a FastAPI dependency that validates JWT Bearer tokens or cookies.

    When secret is empty, auth is disabled (all requests pass as anonymous).
    This allows compose-smoke CI tests to run without token generation.

    Auth resolution order:
    1. Authorization: Bearer <token> header (takes precedence)
    2. access_token cookie (set by Go auth service as httpOnly cookie)
    """
    if not secret:

        async def no_auth(
            credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
        ) -> str:
            return "anonymous"

        return no_auth

    async def require_auth(
        request: Request,
        credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    ) -> str:
        """Validate JWT from Bearer header or cookie and return userId."""
        token: str | None = None

        # Bearer header takes precedence over cookie
        if credentials is not None:
            token = credentials.credentials
        else:
            # Fall back to access_token cookie (set by Go auth service)
            token = request.cookies.get("access_token")

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
        return user_id

    return require_auth
