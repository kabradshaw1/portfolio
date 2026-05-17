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


def test_settings_validate_rejects_rerank_candidate_limit_below_top_k():
    settings = Settings(top_k=10, rerank_candidate_limit=9)

    with pytest.raises(ValueError, match="rerank_candidate_limit"):
        settings.validate()


def test_settings_validate_rejects_rerank_max_candidates_below_candidate_limit():
    settings = Settings(rerank_candidate_limit=20, rerank_max_candidates=19)

    with pytest.raises(ValueError, match="rerank_max_candidates"):
        settings.validate()


def test_settings_include_rate_limit_and_internal_eval_defaults():
    settings = Settings()

    assert settings.chat_rate_limit_ask_operator == "60/minute"
    assert settings.chat_rate_limit_ask_user == "20/minute"
    assert settings.chat_rate_limit_ask_anonymous == "5/minute"
    assert settings.chat_rate_limit_ask_internal_eval == "60/minute"
    assert settings.chat_rate_limit_search_operator == "120/minute"
    assert settings.chat_rate_limit_search_user == "30/minute"
    assert settings.chat_rate_limit_search_anonymous == "10/minute"
    assert settings.chat_rate_limit_search_internal_eval == "120/minute"
    assert settings.rag_internal_eval_token == ""
