package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
)

const maxRewrites = 4

const analyzePrompt = `A reader asked this question about a book on Go:

%s

Return:
- features: at most 8 ` + featureRule + `.
- rewrites: at most 4 other ways to ask for the same information, each using
  different words: one in the terms the book would likely use, one as search
  keywords, one more specific, one more general.`

const analyzeSchema = `{
	"type": "object",
	"properties": {
		"features": {"type": "array", "items": {"type": "string"}},
		"rewrites": {"type": "array", "items": {"type": "string"}}
	},
	"required": ["features", "rewrites"]
}`

type analysis struct {
	Question string   `json:"-"`
	Features []string `json:"features"`
	Rewrites []string `json:"rewrites"`
}

func analyze(ctx context.Context, chat *kronk.Kronk, question string) (analysis, error) {
	a, err := chatJSON[analysis](ctx, chat, fmt.Sprintf(analyzePrompt, question), analyzeSchema)
	if err != nil {
		return analysis{}, fmt.Errorf("analyze: %w", err)
	}

	a.Question = question
	a.Features = clean(a.Features, maxFeatures, normalize)
	a.Rewrites = clean(a.Rewrites, maxRewrites, strings.TrimSpace)

	fmt.Println("\n--- analyze ---")
	gray("features : %s\n", strings.Join(a.Features, ", "))
	for i, r := range a.Rewrites {
		gray("rewrite %d: %s\n", i+1, r)
	}

	return a, nil
}
