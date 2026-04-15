"""JWT authentication dependency for FastAPI services."""

import jwt
from fastapi import Depends, HTTPException
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

_bearer_scheme = HTTPBearer(auto_error=False)


def create_auth_dependency(secret: str):
    """Create a FastAPI dependency that validates JWT Bearer tokens.

    When secret is empty, auth is disabled (all requests pass as anonymous).
    This allows compose-smoke CI tests to run without token generation.
    """
    if not secret:

        async def no_auth(
            credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
        ) -> str:
            return "anonymous"

        return no_auth

    async def require_auth(
        credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    ) -> str:
        """Validate JWT and return userId."""
        if credentials is None:
            raise HTTPException(status_code=401, detail="Missing authorization")
        token = credentials.credentials
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
