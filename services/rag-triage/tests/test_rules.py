from app.models import QueryResult, Scores
from app.rules import classify_case, cluster_cases, recommendations_for_clusters


def result(scores: Scores) -> QueryResult:
    return QueryResult(query="What is supported?", answer="Answer", scores=scores)


def test_low_context_recall_classifies_retrieval_recall():
    diagnosis = classify_case(
        result(Scores(context_recall=0.2, context_precision=0.8, faithfulness=0.4))
    )

    assert diagnosis.failure_mode == "retrieval_recall"
    assert diagnosis.confidence == "high"
    assert "recall" in diagnosis.rationale


def test_low_precision_with_recall_classifies_retrieval_precision():
    diagnosis = classify_case(
        result(Scores(context_recall=0.8, context_precision=0.25, faithfulness=0.7))
    )

    assert diagnosis.failure_mode == "retrieval_precision"
    assert diagnosis.confidence == "high"


def test_low_faithfulness_with_context_classifies_generation():
    diagnosis = classify_case(
        result(Scores(context_recall=0.8, context_precision=0.8, faithfulness=0.2))
    )

    assert diagnosis.failure_mode == "generation_faithfulness"


def test_low_relevancy_classifies_answer_relevance():
    diagnosis = classify_case(
        result(Scores(answer_relevancy=0.2, context_recall=0.9, context_precision=0.9))
    )

    assert diagnosis.failure_mode == "answer_relevance"


def test_missing_scores_classifies_insufficient_evidence():
    diagnosis = classify_case(result(Scores()))

    assert diagnosis.failure_mode == "insufficient_evidence"
    assert diagnosis.confidence == "low"


def test_clusters_and_recommendations_are_structured():
    cases = [
        classify_case(result(Scores(context_recall=0.1))),
        classify_case(result(Scores(context_recall=0.2))),
    ]

    clusters = cluster_cases(cases)
    recommendations = recommendations_for_clusters(clusters)

    assert clusters[0].failure_mode == "retrieval_recall"
    assert clusters[0].count == 2
    assert recommendations[0].action == "increase_top_k"
