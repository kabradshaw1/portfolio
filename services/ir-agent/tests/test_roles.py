from app.roles import model_for


class _RecordingBuilder:
    def __init__(self):
        self.calls = []

    def __call__(self, **kwargs):
        self.calls.append(kwargs)
        return kwargs  # stand-in for a chat model


def test_model_for_uses_correct_model_per_role():
    b = _RecordingBuilder()
    triage = model_for("triage", builder=b)
    report = model_for("report", builder=b)
    assert triage["model"] == "claude-haiku-4-5"
    assert report["model"] == "claude-sonnet-4-6"


def test_model_for_passes_api_key():
    b = _RecordingBuilder()
    model_for("investigate", builder=b)
    assert "api_key" in b.calls[0]
