package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	contextBudget = 6000
	minRelevance  = 0.10
)

const answerPrompt = `Answer the question using only the context.
Each context section starts with a source id like [#12]. Cite the ids you use.
If the context does not answer the question, say so.`

// fit keeps the best chunks that clear minRelevance until the token budget is
// spent. Nothing relevant means the model is never asked.
func fit(docs []document) []document {
	var out []document
	var used int

	for _, d := range docs {
		if d.Relevance < minRelevance {
			break
		}

		tokens := countTokens(d.Text)
		if used+tokens > contextBudget {
			continue
		}

		used += tokens
		out = append(out, d)
	}

	gray("context: %d chunks, %d of %d tokens\n", len(out), used, contextBudget)
	explain(out)

	return out
}

func answer(ctx context.Context, chat *kronk.Kronk, question string, docs []document) error {
	if len(docs) == 0 {
		fmt.Println("\nMODEL> The book does not answer that.")
		return nil
	}

	var prompt strings.Builder
	for _, d := range docs {
		fmt.Fprintf(&prompt, "[#%d] %s\n%s\n\n", d.ID, d.Section, d.Text)
	}

	ch, err := chat.ChatStreaming(ctx, model.D{
		"messages": []model.D{
			model.TextMessage("system", answerPrompt),
			model.TextMessage("user", fmt.Sprintf("Context:\n\n%s\nQuestion: %s", prompt.String(), question)),
		},
		"enable_thinking": false,
		"max_tokens":      2048,
		"temperature":     0.3,
	})
	if err != nil {
		return fmt.Errorf("answer: %w", err)
	}

	fmt.Print("\nMODEL> ")

	for resp := range ch {
		if len(resp.Choices) == 0 || resp.Choices[0].Delta == nil {
			continue
		}

		switch c := resp.Choices[0]; c.FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("answer: %s", c.Delta.Content)
		case model.FinishReasonStop, model.FinishReasonLength:
		default:
			fmt.Print(c.Delta.Content)
		}
	}

	fmt.Println()

	return nil
}
