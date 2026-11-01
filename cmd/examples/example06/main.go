// This example shows you how to use MongoDB and Kronk to perform a vector
// search for a user question. Before searching, the Qwen model extracts the Go
// features and concepts the user is asking about, and only those are embedded.
// A raw question carries filler ("can you tell me how...") that pulls its
// vector away from the book chunks, so embedding the distilled features gives
// a closer match. The search will return the top 2 chunks from the database.
// Then these chunks are sent to the Qwen model, along with the original
// question, to create a coherent response. You must run example05 first.
//
// # Running the example:
//
//	$ make example06
//
// # This requires running the following commands:
//
//  $ make compose-up
//  $ make kronk-up
//	$ make example05

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/practical-ai-golab2026/foundation/client"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	urlChat    = "http://localhost:11435/v1/chat/completions"
	urlEmbed   = "http://localhost:11435/v1/embeddings"
	modelChat  = "unsloth/Qwen3-8B-Q8_0"
	modelEmbed = "ggml-org/embeddinggemma-300m-qat-Q8_0"

	dbName  = "example06"
	colName = "book"
)

func init() {
	if v := os.Getenv("LLM_CHAT_SERVER"); v != "" {
		urlChat = v
	}

	if v := os.Getenv("LLM_EMBED_SERVER"); v != "" {
		urlEmbed = v
	}

	if v := os.Getenv("LLM_CHAT_MODEL"); v != "" {
		modelChat = v
	}

	if v := os.Getenv("LLM_EMBED_MODEL"); v != "" {
		modelEmbed = v
	}
}

// =============================================================================

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nAsk Bill a question about Go: ")

	question, _ := reader.ReadString('\n')
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}

	fmt.Print("\n")

	features, err := extractFeatures(ctx, question)
	if err != nil {
		return fmt.Errorf("extractFeatures: %w", err)
	}

	fmt.Printf("Searching for: %s\n\n", features)

	results, err := vectorSearch(ctx, features)
	if err != nil {
		return fmt.Errorf("vectorSearch: %w", err)
	}

	if err := questionResponse(ctx, question, results); err != nil {
		return fmt.Errorf("questionResponse: %w", err)
	}

	return nil
}

// extractFeatures asks the chat model to reduce the user's question to the Go
// features and concepts it is about. That text, not the raw question, is what
// gets embedded for the vector search.
func extractFeatures(ctx context.Context, question string) (string, error) {
	const prompt = `Extract the Go programming language features and concepts that the
	QUESTION is asking about.

	Rules:
	- Respond with at most %d comma-separated terms on a single line and nothing else.
	- Use the terminology a Go book would use (for example "goroutines",
	  "channels", "interfaces", "method sets", "escape analysis").
	- Do not answer the question and do not explain your choices.

	QUESTION:
	%s
	/no_think`

	// A short list keeps the embedding focused. An open-ended list lets the
	// model drift into loosely related terms, and eventually into a loop.
	const maxFeatures = 8

	// This should take well under a second. Give it its own deadline so a
	// runaway generation fails fast instead of eating the whole run's budget.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	llm := client.NewLLM(urlChat, modelChat)

	// Qwen3's recommended non-thinking sampling plus a repeat penalty. Greedy
	// decoding (temperature 0) makes Qwen3 repeat itself until it times out.
	answer, err := llm.ChatCompletions(ctx, fmt.Sprintf(prompt, maxFeatures, question),
		client.WithParams(0.7, 0.8, 20),
		client.WithRepeatPenalty(1.1, 64),
	)
	if err != nil {
		return "", fmt.Errorf("chat completions: %w", err)
	}

	// Qwen3 may emit a <think>...</think> block before its answer even when
	// asked not to. Only the text after it is the list of features.
	if _, after, found := strings.Cut(answer, "</think>"); found {
		answer = after
	}

	// Don't trust the model to respect the limit: normalize, drop empty and
	// duplicate terms, and cap the list ourselves.
	var features []string
	for term := range strings.SplitSeq(answer, ",") {
		term = strings.Join(strings.Fields(term), " ")
		if term == "" || slices.Contains(features, term) {
			continue
		}

		features = append(features, term)
		if len(features) == maxFeatures {
			break
		}
	}

	// If the model came back with nothing usable, searching with the original
	// question is still better than searching with an empty string.
	if len(features) == 0 {
		return question, nil
	}

	return strings.Join(features, ", "), nil
}

func vectorSearch(ctx context.Context, features string) ([]searchResult, error) {
	llm := client.NewLLM(urlEmbed, modelEmbed)

	vector, err := llm.EmbedText(ctx, features)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}

	// -------------------------------------------------------------------------

	client, err := mongodb.Connect(ctx, "mongodb://localhost:27017", "ardan", "ardan")
	if err != nil {
		return nil, fmt.Errorf("mongodb.Connect: %w", err)
	}
	defer client.Disconnect(ctx)

	col := client.Database(dbName).Collection(colName)

	// -------------------------------------------------------------------------

	const limitResults = 2

	results, err := vectorDBSearch(ctx, col, vector, limitResults)
	if err != nil {
		return nil, fmt.Errorf("vectorDBSearch: %w", err)
	}

	return results, nil
}

func questionResponse(ctx context.Context, question string, results []searchResult) error {
	const prompt = `Use only the CONTEXT to answer the user's question.	
	
	If the CONTEXT doesn't provide enough context, say that you don't know.	
	
	Answer the question and provide additional helpful information.
	
	Responses should be properly formatted to be easily read.	
	
	CONTEXT:
	%s	
	
	QUESTION:
	%s
`

	var chunks strings.Builder

	for _, res := range results {
		if res.Score >= .70 {
			chunks.WriteString(res.Text)
			chunks.WriteString(".\n")

			// YOU WILL WANT TO KNOW HOW MANY TOKENS ARE CURRENTLY IN THE CHUNK
			// SO YOU DON'T EXCEED THE CONTEXT WINDOW (MAXIMUM TOKENS ALLOWED BY
			// THE MODEL). OUR CURRENT MODEL SUPPORTS 8192 TOKENS. THERE IS A
			// TIKTOKEN PACKAGE IN FOUNDATION TO HELP YOU WITH THIS.
		}
	}

	content := chunks.String()
	if content == "" {
		fmt.Println("Don't have enough information to provide an answer")
		return nil
	}

	finalPrompt := fmt.Sprintf(prompt, content, question)

	// -------------------------------------------------------------------------

	llm := client.NewLLM(urlChat, modelChat)

	ch, err := llm.ChatCompletionsSSE(ctx, finalPrompt)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}

	fmt.Print("Model Response:\n\n")

	for resp := range ch {
		// A chunk can arrive with no choices at all, so never index blindly.
		if len(resp.Choices) == 0 {
			continue
		}

		fmt.Print(resp.Choices[0].Delta.Content)
	}

	return nil
}

// =============================================================================

type searchResult struct {
	ID        int       `bson:"id"`
	Text      string    `bson:"text"`
	Embedding []float64 `bson:"embedding"`
	Score     float64   `bson:"score"`
}

func vectorDBSearch(ctx context.Context, col *mongo.Collection, vector []float64, limit int) ([]searchResult, error) {
	pipeline := mongo.Pipeline{
		{{
			Key: "$vectorSearch",
			Value: bson.M{
				"index":       "vector_index",
				"exact":       true,
				"path":        "embedding",
				"queryVector": vector,
				"limit":       limit,
			}},
		},
		{{
			Key: "$project",
			Value: bson.M{
				"id":        1,
				"text":      1,
				"embedding": 1,
				"score": bson.M{
					"$meta": "vectorSearchScore",
				},
			}},
		},
	}

	cur, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate: %w", err)
	}
	defer cur.Close(ctx)

	var results []searchResult
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("all: %w", err)
	}

	return results, nil
}
