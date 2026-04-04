from app.config import Settings


def test_default_settings():
    s = Settings()
    assert s.ollama_base_url == "http://host.docker.internal:11434"
    assert s.chat_model == "qwen2.5:14b"
    assert s.embedding_model == "nomic-embed-text"
    assert s.qdrant_host == "qdrant"
    assert s.qdrant_port == 6333
    assert s.max_agent_steps == 10
    assert s.max_file_lines == 100
    assert s.max_grep_matches == 20
    assert s.test_timeout_seconds == 30


def test_settings_from_env(monkeypatch):
    monkeypatch.setenv("CHAT_MODEL", "mistral")
    monkeypatch.setenv("MAX_AGENT_STEPS", "5")
    s = Settings()
    assert s.chat_model == "mistral"
    assert s.max_agent_steps == 5
