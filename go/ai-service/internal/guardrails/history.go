package guardrails

import (
	"log/slog"
	"strings"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
)

const DefaultMaxHistory = 20

// TruncateHistory keeps at most n most-recent messages. If the first message
// is a system message, it is preserved regardless of the cap.
func TruncateHistory(msgs []llm.Message, n int) []llm.Message {
	if len(msgs) <= n {
		return msgs
	}
	dropped := len(msgs) - n
	slog.Info("history truncated",
		"messages_before", len(msgs),
		"messages_after", n,
		"dropped", dropped,
	)
	if len(msgs) == 0 || msgs[0].Role != llm.RoleSystem {
		return append([]llm.Message(nil), msgs[len(msgs)-n:]...)
	}
	out := make([]llm.Message, 0, n)
	out = append(out, msgs[0])
	out = append(out, msgs[len(msgs)-(n-1):]...)
	return out
}

var refusalPrefixes = []string{
	"i can't",
	"i cannot",
	"i'm not able",
	"i am not able",
	"i'm unable",
	"sorry, i can",
}

// IsRefusal returns true if text looks like a model refusal. Used for metric
// tagging and per-turn logging, not for user-facing behavior.
func IsRefusal(text string) bool {
	low := strings.TrimSpace(strings.ToLower(text))
	for _, p := range refusalPrefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}
