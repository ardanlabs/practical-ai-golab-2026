package main

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// rerank reads the question and each candidate together in one forward pass.
// Embeddings were computed apart from the question; this is the first step that
// judges a chunk against what was actually asked.
func rerank(ctx context.Context, reranker *kronk.Kronk, question string, docs []document) error {
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Text
	}

	resp, err := reranker.Rerank(ctx, model.D{"query": question, "documents": texts})
	if err != nil {
		return fmt.Errorf("rerank: %w", err)
	}

	for _, r := range resp.Data {
		docs[r.Index].Relevance = float64(r.RelevanceScore)
	}

	slices.SortStableFunc(docs, func(a, b document) int {
		return cmp.Compare(b.Relevance, a.Relevance)
	})

	fmt.Println("--- rerank ---")
	gray("%-5s %-6s %-6s %-30s %s\n", "id", "score", "fused", "section", "found by")
	for _, d := range docs {
		gray("#%-4d %.3f  %-6d %-30s %s\n",
			d.ID, d.Relevance, d.FusedRank, oneLine(d.Section, 30), foundBy(d.Hits))
	}

	return nil
}
