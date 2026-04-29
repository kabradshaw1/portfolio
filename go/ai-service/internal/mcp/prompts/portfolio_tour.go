package prompts

import "context"

type portfolioTour struct{}

// NewPortfolioTour returns the tell-me-about-this-portfolio prompt: a
// guided tour grounded in the runbook and schema resources.
func NewPortfolioTour() Prompt { return portfolioTour{} }

func (portfolioTour) Name() string { return "tell-me-about-this-portfolio" }
func (portfolioTour) Description() string {
	return "Guided tour of how this portfolio is built."
}
func (portfolioTour) Arguments() []Argument { return nil }
func (portfolioTour) Render(_ context.Context, _ map[string]string) (Rendered, error) {
	return Rendered{
		Description: "Tour of the portfolio architecture.",
		Messages: []Message{
			{Role: "system", Text: "You are giving a guided tour of a software engineering portfolio."},
			{Role: "user", Text: "Read the resource at runbook://how-this-portfolio-works and the resource at schema://ecommerce, then walk me through how the AI services, ecommerce services, and data layer fit together. Aim for 4 short sections."},
		},
	}, nil
}
