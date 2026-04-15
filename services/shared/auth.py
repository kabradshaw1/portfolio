"""JWT authentication dependency for FastAPI services."""

import jwt
from fastapi import Depends, HTTPException
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

_bearer_scheme = HTTPBearer()


def create_auth_dependency(secret: str):
    """Create a FastAPI dependency that validates JWT Bearer tokens."""

    async def require_auth(
        credentials: HTTPAuthorizationCredentials = Depends(_bearer_scheme),
    ) -> str:
        """Validate JWT and return userId."""
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
