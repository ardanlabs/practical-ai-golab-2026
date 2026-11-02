package main

import (
	"fmt"
	"slices"
	"strings"
)

var kinds = []string{"text", "summary", "topic", "feature", "next"}

// foundBy summarizes a chunk's hits for the rerank table: "text×5 topic×2 next".
func foundBy(hits []hit) string {
	var parts []string
	for _, k := range kinds {
		switch n := len(ofKind(hits, k)); n {
		case 0:
		case 1:
			parts = append(parts, k)
		default:
			parts = append(parts, fmt.Sprintf("%s×%d", k, n))
		}
	}

	return strings.Join(parts, " ")
}

// explain prints, for every chunk in the context, each search that found it,
// its rank there, and what matched.
func explain(docs []document) {
	fmt.Println("--- where the context came from ---")
	gray("q is the question, r1-r4 the rewrites, #n the rank in that search\n")

	for _, d := range docs {
		gray("#%-4d %s, rerank %.3f, fused #%d\n", d.ID, d.Section, d.Relevance, d.FusedRank)

		for _, k := range kinds {
			hits := ofKind(d.Hits, k)
			if len(hits) == 0 {
				continue
			}

			var ranks, whys []string
			for _, h := range hits {
				if h.Rank > 0 {
					ranks = append(ranks, fmt.Sprintf("%s#%d", h.Query, h.Rank))
				}
				if h.Why != "" && !slices.Contains(whys, h.Why) {
					whys = append(whys, h.Why)
				}
			}

			gray("      %-8s %-24s %s\n", k, strings.Join(ranks, " "), oneLine(strings.Join(whys, " | "), 80))
		}
	}
}

func ofKind(hits []hit, kind string) []hit {
	var out []hit
	for _, h := range hits {
		if h.Kind == kind {
			out = append(out, h)
		}
	}

	return out
}
