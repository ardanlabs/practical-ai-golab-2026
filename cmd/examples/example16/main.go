// This example is a complete RAG application over the Ultimate Go Notebook.
// Postgres holds the vectors (pgvector) and the graph (Apache AGE), and the
// Kronk SDK runs every model.
//
// Ingest rebuilds everything from zarf/data/book.txt:
//
//	chunk    split the book by section, with overlap         chunk.go
//	enrich   one LLM call per chunk: summary, features,      enrich.go
//	         search topics
//	store    three vector tables and a graph                 store.go
//
// Ask answers questions against what ingest built:
//
//	analyze  one LLM call: features and rewrites             analyze.go
//	recall   every phrasing against every table, plus graph  recall.go
//	fuse     reciprocal rank fusion, plus NEXT neighbours    fuse.go
//	rerank   a cross-encoder reads question and chunk        rerank.go
//	answer   token budget, citations, or "not in the book"   answer.go
//
// Baseline answers with plain top-k vector search, for contrast.
//
// # Running the example:
//
//	$ make compose-up
//	$ make example16-ingest
//	$ make example16-baseline   # This shows a simple RAG pipeline
//	$ make example16            # This shows you the full-blown features

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	modelChat   = "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL"
	modelEmbed  = "ggml-org/embeddinggemma-300m-qat-Q8_0"
	modelRerank = "gpustack/bge-reranker-v2-m3-Q8_0"
	bookFile    = "zarf/data/book.txt"
	dimensions  = 768
)

func main() {
	mode := "ask"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	if err := run(context.Background(), mode); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, mode string) error {
	switch mode {
	case "ingest":
		return ingest(ctx)
	case "ask":
		return ask(ctx, false)
	case "baseline":
		return ask(ctx, true)
	}

	return fmt.Errorf("unknown mode %q: want ingest, ask or baseline", mode)
}

func ingest(ctx context.Context) error {
	embedder, err := loadModel(ctx, modelEmbed)
	if err != nil {
		return err
	}
	defer embedder.Unload(context.WithoutCancel(ctx))

	chat, err := loadModel(ctx, modelChat, model.WithContextWindow(8192))
	if err != nil {
		return err
	}
	defer chat.Unload(context.WithoutCancel(ctx))

	g, err := openGraph(ctx)
	if err != nil {
		return err
	}
	defer g.Close()

	chunks, err := chunkBook(bookFile)
	if err != nil {
		return err
	}

	return store(ctx, g, embedder, chat, chunks)
}

func ask(ctx context.Context, baseline bool) error {
	embedder, err := loadModel(ctx, modelEmbed)
	if err != nil {
		return err
	}
	defer embedder.Unload(context.WithoutCancel(ctx))

	reranker, err := loadModel(ctx, modelRerank)
	if err != nil {
		return err
	}
	defer reranker.Unload(context.WithoutCancel(ctx))

	chat, err := loadModel(ctx, modelChat, model.WithContextWindow(32768))
	if err != nil {
		return err
	}
	defer chat.Unload(context.WithoutCancel(ctx))

	g, err := openGraph(ctx)
	if err != nil {
		return err
	}
	defer g.Close()

	hubs, err := hubFeatures(ctx, g)
	if err != nil {
		return err
	}

	fmt.Printf("\nTry: %q\n", "How does the scheduler handle blocking system calls?")

	input := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\nUSER> ")
		if !input.Scan() {
			return input.Err()
		}

		question := strings.TrimSpace(input.Text())
		switch question {
		case "":
			continue
		case "quit":
			return nil
		}

		if baseline {
			docs, err := vectorOnly(ctx, g, embedder, question)
			if err != nil {
				return err
			}

			if err := answer(ctx, chat, question, docs); err != nil {
				return err
			}

			continue
		}

		a, err := analyze(ctx, chat, question)
		if err != nil {
			return err
		}

		lists, err := recall(ctx, g, embedder, a, hubs)
		if err != nil {
			return err
		}

		docs, err := fuse(ctx, g, lists)
		if err != nil {
			return err
		}

		if err := rerank(ctx, reranker, question, docs); err != nil {
			return err
		}

		if err := answer(ctx, chat, question, fit(docs)); err != nil {
			return err
		}
	}
}
