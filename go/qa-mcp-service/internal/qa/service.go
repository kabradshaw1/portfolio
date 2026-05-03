package qa

import (
	"context"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/content"
	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/store"
)

type Store interface {
	UpsertQuestions(context.Context, []content.Question) error
	CreateSession(context.Context) (int64, error)
	ListTopics(context.Context) ([]store.Topic, error)
	NextQuestion(context.Context, store.QuestionFilter) (store.Question, error)
	SubmitAnswer(context.Context, store.SubmitAnswerInput) (store.AnswerAttempt, error)
	RecordFeedback(context.Context, store.FeedbackInput) error
	UpdateExpectedAnswer(context.Context, int64, string) error
	ProgressSummary(context.Context) (store.ProgressSummary, error)
}

type Service struct {
	store        Store
	materialPath string
	sessionID    int64
}

type ImportResult struct {
	ImportedQuestions int `json:"imported_questions"`
}

type AnswerReview struct {
	AttemptID      int64  `json:"attempt_id"`
	QuestionID     int64  `json:"question_id"`
	Answer         string `json:"answer"`
	ExpectedAnswer string `json:"expected_answer"`
}

type FeedbackInput struct {
	AttemptID        int64  `json:"attempt_id"`
	Score            int    `json:"score"`
	MissingPoints    string `json:"missing_points"`
	InaccuratePoints string `json:"inaccurate_points"`
	SuggestedAnswer  string `json:"suggested_answer"`
}

func New(store Store, materialPath string) *Service {
	return &Service{store: store, materialPath: materialPath}
}

func (s *Service) ImportMaterial(ctx context.Context) (ImportResult, error) {
	questions, err := content.ParseDir(s.materialPath)
	if err != nil {
		return ImportResult{}, err
	}
	if err := s.store.UpsertQuestions(ctx, questions); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{ImportedQuestions: len(questions)}, nil
}

func (s *Service) ListTopics(ctx context.Context) ([]store.Topic, error) {
	return s.store.ListTopics(ctx)
}

func (s *Service) GetNextQuestion(ctx context.Context, filter store.QuestionFilter) (store.Question, error) {
	return s.store.NextQuestion(ctx, filter)
}

func (s *Service) SubmitAnswer(ctx context.Context, questionID int64, answer string) (AnswerReview, error) {
	if questionID <= 0 {
		return AnswerReview{}, fmt.Errorf("question_id is required")
	}
	if answer == "" {
		return AnswerReview{}, fmt.Errorf("answer is required")
	}
	if s.sessionID == 0 {
		sessionID, err := s.store.CreateSession(ctx)
		if err != nil {
			return AnswerReview{}, err
		}
		s.sessionID = sessionID
	}
	attempt, err := s.store.SubmitAnswer(ctx, store.SubmitAnswerInput{
		SessionID:  s.sessionID,
		QuestionID: questionID,
		Answer:     answer,
	})
	if err != nil {
		return AnswerReview{}, err
	}
	return AnswerReview{
		AttemptID:      attempt.ID,
		QuestionID:     attempt.QuestionID,
		Answer:         attempt.Answer,
		ExpectedAnswer: attempt.ExpectedAnswerSnapshot,
	}, nil
}

func (s *Service) RecordFeedback(ctx context.Context, input FeedbackInput) error {
	if input.AttemptID <= 0 {
		return fmt.Errorf("attempt_id is required")
	}
	if input.Score < 0 || input.Score > 3 {
		return fmt.Errorf("score must be between 0 and 3")
	}
	return s.store.RecordFeedback(ctx, store.FeedbackInput{
		AttemptID:        input.AttemptID,
		Score:            input.Score,
		MissingPoints:    input.MissingPoints,
		InaccuratePoints: input.InaccuratePoints,
		SuggestedAnswer:  input.SuggestedAnswer,
	})
}

func (s *Service) ProgressSummary(ctx context.Context) (store.ProgressSummary, error) {
	return s.store.ProgressSummary(ctx)
}

func (s *Service) UpdateExpectedAnswer(ctx context.Context, questionID int64, answer string) error {
	if questionID <= 0 {
		return fmt.Errorf("question_id is required")
	}
	if answer == "" {
		return fmt.Errorf("answer is required")
	}
	return s.store.UpdateExpectedAnswer(ctx, questionID, answer)
}
