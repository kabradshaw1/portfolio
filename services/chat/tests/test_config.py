import pytest
from app.config import Settings


def test_settings_validate_rejects_invalid_retrieval_mode():
    settings = Settings(retrieval_mode="keyword")

    with pytest.raises(ValueError, match="retrieval_mode"):
        settings.validate()


def test_settings_validate_rejects_hybrid_prefetch_limit_below_top_k():
    settings = Settings(top_k=10, hybrid_prefetch_limit=9)

    with pytest.raises(ValueError, match="hybrid_prefetch_limit"):
        settings.validate()
