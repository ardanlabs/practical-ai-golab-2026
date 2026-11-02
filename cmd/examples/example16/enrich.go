package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
)

const (
	maxFeatures = 8
	maxTopics   = 5
)

// Used by enrich and analyze, so the features of a chunk and the features of a
// question are named the same way and meet in the graph.
const featureRule = "technical subjects, each named by the shortest common term, lowercase and singular"

const enrichPrompt = `Read this passage from a book about Go.

Return:
- summary: two or three sentences on what the passage explains.
- features: at most 8 ` + featureRule + `.
- topics: at most 5 short phrases a reader would type into a search box to find this passage.

Section: %s

Passage:
%s`

const enrichSchema = `{
	"type": "object",
	"properties": {
		"summary":  {"type": "string"},
		"features": {"type": "array", "items": {"type": "string"}},
		"topics":   {"type": "array", "items": {"type": "string"}}
	},
	"required": ["summary", "features", "topics"]
}`

type enrichment struct {
	Summary  string   `json:"summary"`
	Features []string `json:"features"`
	Topics   []string `json:"topics"`
}

func enrich(ctx context.Context, chat *kronk.Kronk, c chunk) (enrichment, error) {
	e, err := chatJSON[enrichment](ctx, chat, fmt.Sprintf(enrichPrompt, c.Section, c.Text), enrichSchema)
	if err != nil {
		return enrichment{}, err
	}

	e.Features = clean(e.Features, maxFeatures, normalize)
	e.Topics = clean(e.Topics, maxTopics, strings.TrimSpace)

	return e, nil
}

// clean applies the caps the prompt asked for, since a grammar cannot count.
func clean(items []string, limit int, norm func(string) string) []string {
	var out []string
	for _, s := range items {
		if s = norm(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}

	return out[:min(len(out), limit)]
}

var nameRE = regexp.MustCompile(`[^a-z0-9 ._/-]+`)

// normalize keeps "Goroutines", "goroutine" and "a goroutine" from becoming
// three graph nodes that never meet.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = nameRE.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")

	for _, article := range []string{"the ", "a ", "an "} {
		s = strings.TrimPrefix(s, article)
	}

	if len(s) < 2 || len(s) > 40 {
		return ""
	}

	return s
}
