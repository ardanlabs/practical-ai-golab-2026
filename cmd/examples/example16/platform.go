package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/age"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/tiktoken"
)

var installLibs = sync.OnceValue(func() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	l, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return err
	}

	_, err = l.Download(ctx, kronk.FmtLogger)
	return err
})

func loadModel(ctx context.Context, source string, opts ...model.Option) (*kronk.Kronk, error) {
	if err := installLibs(); err != nil {
		return nil, fmt.Errorf("install llama.cpp: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return nil, err
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, source)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", source, err)
	}

	if err := kronk.Init(); err != nil {
		return nil, err
	}

	krn, err := kronk.New(append([]model.Option{model.WithModelFiles(mp.ModelFiles)}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", source, err)
	}

	return krn, nil
}

func openGraph(ctx context.Context) (*age.Graph, error) {
	return age.Open(ctx, age.Config{
		User:         "postgres",
		Password:     "postgres",
		Host:         "localhost:5433",
		Name:         "postgres",
		GraphName:    "book",
		MaxIdleConns: 4,
		MaxOpenConns: 8,
		DisableTLS:   true,
	})
}

// EmbeddingGemma was trained with these prefixes. Without them a question and
// the passage that answers it land further apart.
func asQuery(text string) string           { return "task: search result | query: " + text }
func asDocument(title, text string) string { return "title: " + title + " | text: " + text }

func embed(ctx context.Context, krn *kronk.Kronk, texts ...string) ([][]float64, error) {
	resp, err := krn.Embeddings(ctx, model.D{"input": texts, "truncate": true})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	out := make([][]float64, len(texts))
	for _, d := range resp.Data {
		v := make([]float64, len(d.Embedding))
		for i, f := range d.Embedding {
			v[i] = float64(f)
		}
		out[d.Index] = v
	}

	return out, nil
}

// chatJSON asks for JSON matching schema. The schema becomes a grammar, so the
// model cannot reply with anything else.
func chatJSON[T any](ctx context.Context, krn *kronk.Kronk, prompt string, schema string) (T, error) {
	var out T

	resp, err := krn.Chat(ctx, model.D{
		"messages":        []model.D{model.TextMessage("user", prompt)},
		"json_schema":     json.RawMessage(schema),
		"enable_thinking": false,
		"max_tokens":      1024,
		"temperature":     0.1,
	})
	if err != nil {
		return out, fmt.Errorf("chat: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return out, errors.New("chat: empty response")
	}

	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &out); err != nil {
		return out, fmt.Errorf("chat: decode: %w", err)
	}

	return out, nil
}

var tokenizer = sync.OnceValue(func() *tiktoken.Tiktoken {
	t, err := tiktoken.NewTiktoken()
	if err != nil {
		panic(err)
	}
	return t
})

func countTokens(text string) int {
	return tokenizer().TokenCount(text)
}

func oneLine(text string, width int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= width {
		return text
	}

	return text[:width-3] + "..."
}

func gray(format string, args ...any) {
	fmt.Printf("\033[90m"+format+"\033[0m", args...)
}
