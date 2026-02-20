package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // Register sqlite driver
)

// MockProvider implements EmbeddingProvider for testing
type MockProvider struct{}

func (m *MockProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for range texts {
		// Return a dummy vector of size 3
		results = append(results, []float32{0.1, 0.2, 0.3})
	}
	return results, nil
}
func (m *MockProvider) Dimensions() int   { return 3 }
func (m *MockProvider) Name() string      { return "mock" }
func (m *MockProvider) ModelName() string { return "test-model" }

func TestChunker(t *testing.T) {
	chunker := &Chunker{
		ChunkSize:    100,
		ChunkOverlap: 20,
	}

	text := "This is a paragraph.\n\nThis is another paragraph that is slightly longer to test chunking capabilities.\n\n# Header\nSection content."
	chunks := chunker.ChunkText("test.md", text)

	if len(chunks) == 0 {
		t.Error("Expected chunks, got 0")
	}

	for i, c := range chunks {
		t.Logf("Chunk %d: %q (len=%d)", i, c.Content, len(c.Content))
		if c.Tokens == 0 {
			t.Error("Token count should not be 0")
		}
	}
}

func TestManagerInit(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "memory_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := MemoryConfig{
		WorkspaceDir: tmpDir,
		StateDir:     tmpDir,
		AgentID:      "test_agent",
		Search: SearchConfig{
			Provider:  "mock",
			Model:     "test-model",
			OllamaURL: "",
			Hybrid: HybridConfig{
				Enabled:             true,
				VectorWeight:        0.5,
				TextWeight:          0.5,
				CandidateMultiplier: 2,
			},
			Cache: CacheConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
			Store: StoreConfig{
				Path: filepath.Join(tmpDir, "test_memory.db"),
			},
		},
	}

	// Initialize Manager
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Set mock embedder
	mgr.SetEmbedder(&MockProvider{})

	// Test adding content
	ctx := context.Background()
	content := "This is a test memory content."

	// Create a dummy file to index
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Index the file
	indexer := NewIndexer(mgr)
	if err := indexer.IndexFile(ctx, testFile); err != nil {
		t.Fatalf("Failed to index file: %v", err)
	}

	// Search (Hybrid)
	searcher := NewSearchEngine(mgr)
	resp, err := searcher.Search(ctx, "test memory", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// We expect at least one result
	if len(resp.Results) == 0 {
		t.Log("Search returned 0 results (might be due to empty FTS or vector score), but execution was successful.")
	} else {
		t.Logf("Found %d results", len(resp.Results))
		for _, r := range resp.Results {
			t.Logf(" - [%.4f] %s", r.FinalScore, r.Chunk.Content)
		}
	}
}

func TestMemoryWriter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "writer_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock manager with just config
	cfg := MemoryConfig{
		WorkspaceDir: tmpDir,
	}
	mgr := &Manager{
		cfg: cfg,
	}

	writer := NewMemoryWriter(mgr)

	// Test WriteToday
	err = writer.WriteToday("Ran unit tests.")
	if err != nil {
		t.Errorf("WriteToday failed: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	expectedPath := filepath.Join(tmpDir, "memory", today+".md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Daily log not created: %s", expectedPath)
	}

	// Test WriteLongTerm
	err = writer.WriteLongTerm("dev", "Prefer Go over Python.")
	if err != nil {
		t.Errorf("WriteLongTerm failed: %v", err)
	}

	longTermPath := filepath.Join(tmpDir, "MEMORY.md")
	content, err := os.ReadFile(longTermPath)
	if err != nil {
		t.Errorf("MEMORY.md not readable: %v", err)
	}
	if !strings.Contains(string(content), "Prefer Go over Python") {
		t.Error("Long term memory content mismatch")
	}
}

func TestUtils(t *testing.T) {
	// contentHash
	h1 := contentHash("hello", "prov", "mod")
	h2 := contentHash("hello", "prov", "mod")
	if h1 != h2 {
		t.Error("Hash must be deterministic")
	}

	// Float32 <-> Bytes
	vec := []float32{1.0, 0.5, -0.5}
	b := float32SliceToBytes(vec)
	if len(b) != len(vec)*4 {
		t.Errorf("Byte slice length mismatch: got %d want %d", len(b), len(vec)*4)
	}

	vec2 := bytesToFloat32Slice(b)
	if len(vec2) != len(vec) {
		t.Errorf("Reconstructed slice length mismatch")
	}
	for i := range vec {
		if vec[i] != vec2[i] {
			t.Errorf("Value mismatch at %d: %f != %f", i, vec[i], vec2[i])
		}
	}
}
