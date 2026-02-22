package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"rag-translator/internal/cache"
	"rag-translator/internal/config"
	"rag-translator/internal/filewalker"
	"rag-translator/internal/graph"
	"rag-translator/internal/interpolation"
	"rag-translator/internal/parser"
	"rag-translator/internal/rag"
	"rag-translator/internal/seed"
	"rag-translator/internal/textutil"
	"rag-translator/internal/translation"
	"rag-translator/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Execute runs the CLI application.
func Execute() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	rootCmd := &cobra.Command{
		Use:   "rag-translator",
		Short: "GraphRAG-based game localization tool for 剑侠世界2",
		Long:  "A production-grade GraphRAG translation tool for localizing Chinese wuxia MMORPG games to Vietnamese.",
	}

	rootCmd.AddCommand(ingestCmd())
	rootCmd.AddCommand(translateCmd())
	rootCmd.AddCommand(ingestSeedGitCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func ingestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest <directory>",
		Short: "Parse game files, generate embeddings, and build knowledge graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIngest(args[0])
		},
	}
}

func translateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "translate <input-dir> <output-dir>",
		Short: "Translate game files using GraphRAG pipeline",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranslate(args[0], args[1])
		},
	}
}

func ingestSeedGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest-seed-git <commit_base> <commit_target> <folder>",
		Short: "Extract translation seed corpus from Git diff and ingest into GraphRAG",
		Long: `Extracts source→translated text pairs from Git diffs between two commits.
Parses .lua, .ini, .txt file changes to identify manual translations.
Generates embeddings, updates knowledge graph, and produces a seed corpus file.`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			exportFormat, _ := cmd.Flags().GetString("export")
			exportPath, _ := cmd.Flags().GetString("output")
			return runIngestSeedGit(args[0], args[1], args[2], exportFormat, exportPath)
		},
	}

	cmd.Flags().String("export", "tsv", "Export format: tsv or json")
	cmd.Flags().String("output", "seed_corpus", "Output path for seed corpus (without extension)")

	return cmd
}

// runIngestSeedGit handles the `ingest-seed-git` command.
func runIngestSeedGit(commitBase, commitTarget, folder, exportFormat, exportPath string) error {
	ctx, cancel := setupContext()
	defer cancel()

	cfg := config.Load()

	pgPool, neo4jDriver, err := initDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer pgPool.Close()
	defer neo4jDriver.Close(ctx)

	// Resolve repo root (use current working directory).
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// 1. Extract pairs from Git diff.
	log.Info().
		Str("base", commitBase).
		Str("target", commitTarget).
		Str("folder", folder).
		Msg("Starting seed ingestion from Git")

	gitIngestor := seed.NewGitIngestor()
	entries, err := gitIngestor.IngestFromGit(ctx, repoRoot, commitBase, commitTarget, folder)
	if err != nil {
		return fmt.Errorf("git ingestion: %w", err)
	}

	if len(entries) == 0 {
		log.Warn().Msg("No translation pairs found in Git diff")
		return nil
	}

	log.Info().Int("pairs", len(entries)).Msg("Extracted translation pairs")

	// 2. Initialize stores.
	seedStore := seed.NewSeedStore(pgPool)

	vectorStore := rag.NewVectorStore(pgPool)

	graphSeeder := seed.NewGraphSeeder(neo4jDriver)
	if err := graphSeeder.EnsureSchema(ctx); err != nil {
		return fmt.Errorf("ensure graph seed schema: %w", err)
	}

	// 3. Store seed entries (deduplicated by hash).
	inserted, _, err := seedStore.Upsert(ctx, entries)
	if err != nil {
		return fmt.Errorf("upsert seed entries: %w", err)
	}
	log.Info().Int("inserted", inserted).Msg("Seed entries stored")

	// 4. Generate and store embeddings.
	embeddingClient := rag.NewEmbeddingClient(cfg.GeminiAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
	vectorSeeder := seed.NewVectorSeeder(embeddingClient, vectorStore)
	if err := vectorSeeder.IngestEmbeddings(ctx, entries, cfg.BatchSize); err != nil {
		return fmt.Errorf("ingest seed embeddings: %w", err)
	}

	// 5. Update knowledge graph.
	if err := graphSeeder.UpsertSeedNodes(ctx, entries); err != nil {
		return fmt.Errorf("upsert seed graph nodes: %w", err)
	}

	// 6. Also populate translation cache with seed translations.
	translationCache := cache.NewTranslationCache(pgPool)
	for _, e := range entries {
		if err := translationCache.Set(ctx, e.SourceText, e.TranslatedText); err != nil {
			log.Warn().Err(err).Str("text", textutil.Truncate(e.SourceText, 30)).Msg("Failed to cache seed translation")
		}
	}

	// 7. Export seed corpus.
	switch exportFormat {
	case "json":
		if err := seedStore.ExportJSON(ctx, exportPath+".json"); err != nil {
			return fmt.Errorf("export JSON: %w", err)
		}
	default:
		if err := seedStore.ExportTSV(ctx, exportPath+".tsv"); err != nil {
			return fmt.Errorf("export TSV: %w", err)
		}
	}

	log.Info().
		Int("pairs", len(entries)).
		Int("stored", inserted).
		Str("format", exportFormat).
		Msg("Seed ingestion complete")

	return nil
}

// setupContext creates a cancellable context with signal handling.
func setupContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Warn().Msg("Received shutdown signal, cancelling...")
		cancel()
	}()

	return ctx, cancel
}

// initDependencies creates all shared dependencies and runs migrations.
func initDependencies(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, neo4j.DriverWithContext, error) {
	// PostgreSQL pool.
	pgPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}

	if err := pgPool.Ping(ctx); err != nil {
		pgPool.Close()
		return nil, nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	log.Info().Msg("Connected to PostgreSQL")

	// Neo4j driver.
	neo4jDriver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""))
	if err != nil {
		pgPool.Close()
		return nil, nil, fmt.Errorf("connect Neo4j: %w", err)
	}

	if err := neo4jDriver.VerifyConnectivity(ctx); err != nil {
		pgPool.Close()
		neo4jDriver.Close(ctx)
		return nil, nil, fmt.Errorf("verify Neo4j connectivity: %w", err)
	}
	log.Info().Msg("Connected to Neo4j")

	return pgPool, neo4jDriver, nil
}

// runIngest handles the `ingest` command.
func runIngest(inputDir string) error {
	ctx, cancel := setupContext()
	defer cancel()

	cfg := config.Load()

	pgPool, neo4jDriver, err := initDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer pgPool.Close()
	defer neo4jDriver.Close(ctx)

	// Ensure Neo4j schemas and seed terminology.
	vectorStore := rag.NewVectorStore(pgPool)

	graphBuilder := graph.NewGraphBuilder(neo4jDriver)
	if err := graphBuilder.EnsureSchema(ctx); err != nil {
		return fmt.Errorf("ensure graph schema: %w", err)
	}
	if err := graphBuilder.SeedTerminology(ctx); err != nil {
		return fmt.Errorf("seed terminology: %w", err)
	}

	// Walk and parse files.
	w := filewalker.NewWalker()
	entries, err := w.Walk(inputDir)
	if err != nil {
		return fmt.Errorf("walk input directory: %w", err)
	}

	log.Info().Int("files", len(entries)).Msg("Starting file ingestion")

	// Parse files using worker pool.
	parsePool := worker.NewPool[filewalker.FileEntry, *parser.ParseResult](cfg.WorkerCount,
		func(ctx context.Context, entry filewalker.FileEntry) (*parser.ParseResult, error) {
			return entry.Parser.Parse(entry.Path)
		},
	)

	parseResults := parsePool.Execute(ctx, entries)

	// Collect all unique texts for embedding.
	textSet := make(map[string]struct{})
	var allTexts []string
	var textContexts []string

	for _, pr := range parseResults {
		if pr.Err != nil {
			log.Error().Err(pr.Err).Str("file", pr.Input.Path).Msg("Parse failed")
			continue
		}
		if pr.Result == nil {
			continue
		}

		for _, et := range pr.Result.Texts {
			if _, exists := textSet[et.Text]; exists {
				continue
			}
			textSet[et.Text] = struct{}{}
			allTexts = append(allTexts, et.Text)

			// Build context string.
			var ctxParts []string
			for k, v := range et.Context {
				ctxParts = append(ctxParts, fmt.Sprintf("%s=%s", k, v))
			}
			textContexts = append(textContexts, strings.Join(ctxParts, "; "))

			// Add entity to graph.
			ctxStr := strings.Join(ctxParts, "; ")
			if err := graphBuilder.AddEntityFromText(ctx, et.Text, et.File, ctxStr); err != nil {
				log.Warn().Err(err).Str("text", textutil.Truncate(et.Text, 30)).Msg("Failed to add entity to graph")
			}
		}
	}

	log.Info().Int("unique_texts", len(allTexts)).Msg("Extracted unique texts")

	// Generate embeddings.
	embeddingClient := rag.NewEmbeddingClient(cfg.GeminiAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
	embeddings, err := embeddingClient.EmbedBatch(ctx, allTexts, cfg.BatchSize)
	if err != nil {
		return fmt.Errorf("generate embeddings: %w", err)
	}

	// Store embeddings.
	var records []rag.EmbeddingRecord
	for i, text := range allTexts {
		if i >= len(embeddings) || embeddings[i] == nil {
			continue
		}
		records = append(records, rag.EmbeddingRecord{
			Hash:     textutil.Hash(text),
			Source:   text,
			Context:  textContexts[i],
			FilePath: "",
			Vector:   embeddings[i],
		})
	}

	if err := vectorStore.Store(ctx, records); err != nil {
		return fmt.Errorf("store embeddings: %w", err)
	}

	log.Info().
		Int("files", len(entries)).
		Int("texts", len(allTexts)).
		Int("embeddings", len(records)).
		Msg("Ingestion complete")

	return nil
}

// runTranslate handles the `translate` command.
func runTranslate(inputDir, outputDir string) error {
	ctx, cancel := setupContext()
	defer cancel()

	cfg := config.Load()

	pgPool, neo4jDriver, err := initDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer pgPool.Close()
	defer neo4jDriver.Close(ctx)

	// Initialize components.
	vectorStore := rag.NewVectorStore(pgPool)
	embeddingClient := rag.NewEmbeddingClient(cfg.GeminiAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
	graphQuerier := graph.NewGraphQuerier(neo4jDriver)
	retriever := rag.NewRetriever(vectorStore, embeddingClient, graphQuerier)
	promptBuilder := translation.NewPromptBuilder()
	opusClient := translation.NewOpusClient(cfg.GeminiAPIKey, cfg.TranslationModel)
	translationCache := cache.NewTranslationCache(pgPool)

	// Preload cache.
	if err := translationCache.Preload(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to preload cache")
	}

	// Get terminology map for batch prompts.
	terminologyMap, err := graphQuerier.GetAllTerminology(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load terminology")
		terminologyMap = make(map[string]string)
	}

	// Walk and parse files.
	w := filewalker.NewWalker()
	entries, err := w.Walk(inputDir)
	if err != nil {
		return fmt.Errorf("walk input directory: %w", err)
	}

	log.Info().Int("files", len(entries)).Msg("Starting translation pipeline")

	// Ensure output directory exists.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Parse all files first.
	parsePool := worker.NewPool[filewalker.FileEntry, *parser.ParseResult](cfg.WorkerCount,
		func(ctx context.Context, entry filewalker.FileEntry) (*parser.ParseResult, error) {
			return entry.Parser.Parse(entry.Path)
		},
	)
	parseResults := parsePool.Execute(ctx, entries)

	// Collect deduplicated texts needing translation.
	textSet := make(map[string]struct{})
	var textsToTranslate []string

	for _, pr := range parseResults {
		if pr.Err != nil || pr.Result == nil {
			continue
		}
		for _, et := range pr.Result.Texts {
			if _, exists := textSet[et.Text]; exists {
				continue
			}
			textSet[et.Text] = struct{}{}

			// Check cache.
			if _, cached := translationCache.Get(ctx, et.Text); cached {
				continue
			}

			textsToTranslate = append(textsToTranslate, et.Text)
		}
	}

	log.Info().
		Int("total_unique", len(textSet)).
		Int("to_translate", len(textsToTranslate)).
		Msg("Translation plan")

	// Translate texts in batches with concurrency control.
	semaphore := make(chan struct{}, cfg.MaxConcurrentAPICalls)
	systemPrompt := promptBuilder.GetSystemPrompt()

	batches := worker.Batch(textsToTranslate, cfg.BatchSize)

	for batchIdx, batch := range batches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		semaphore <- struct{}{} // Acquire.

		log.Info().
			Int("batch", batchIdx+1).
			Int("total_batches", len(batches)).
			Int("size", len(batch)).
			Msg("Translating batch")

		// Protect interpolation variables.
		protectedTexts := make([]string, len(batch))
		mappings := make([][]interpolation.Mapping, len(batch))
		for i, text := range batch {
			protectedTexts[i], mappings[i] = interpolation.Protect(text)
		}

		// Build batch prompt with terminology.
		relevantTerms := make(map[string]string)
		for _, text := range batch {
			for zh, vi := range terminologyMap {
				if strings.Contains(text, zh) {
					relevantTerms[zh] = vi
				}
			}
		}

		userPrompt := promptBuilder.BuildBatchUserPrompt(protectedTexts, relevantTerms)

		// Call API.
		response, err := opusClient.Translate(ctx, systemPrompt, userPrompt)
		<-semaphore // Release.

		if err != nil {
			log.Error().Err(err).Int("batch", batchIdx+1).Msg("Batch translation failed")
			continue
		}

		// Parse response.
		parts := strings.Split(response, "|||")
		for i, text := range batch {
			var translated string
			if i < len(parts) {
				translated = strings.TrimSpace(parts[i])
			} else {
				log.Warn().Str("text", textutil.Truncate(text, 30)).Msg("Missing translation in batch response, using fallback")
				// Fallback: try individual translation.
				retrievalResult, _ := retriever.Retrieve(ctx, text, 3)
				protectedText, mapping := interpolation.Protect(text)
				userPrompt := promptBuilder.BuildUserPrompt(protectedText, retriever, retrievalResult)
				individual, err := opusClient.Translate(ctx, systemPrompt, userPrompt)
				if err != nil {
					log.Error().Err(err).Str("text", textutil.Truncate(text, 30)).Msg("Individual translation failed")
					continue
				}
				translated = interpolation.Restore(individual, mapping)
				if err := translationCache.Set(ctx, text, translated); err != nil {
					log.Warn().Err(err).Msg("Failed to cache translation")
				}
				continue
			}

			// Restore interpolation variables.
			translated = interpolation.Restore(translated, mappings[i])

			// Cache the result.
			if err := translationCache.Set(ctx, text, translated); err != nil {
				log.Warn().Err(err).Msg("Failed to cache translation")
			}
		}
	}

	// Reconstruct files with translations.
	inputAbs, _ := filepath.Abs(inputDir)
	outputAbs, _ := filepath.Abs(outputDir)

	for _, pr := range parseResults {
		if pr.Err != nil || pr.Result == nil {
			continue
		}

		// Build translations map for this file.
		fileTranslations := make(map[string]string)
		for _, et := range pr.Result.Texts {
			if translated, ok := translationCache.Get(ctx, et.Text); ok {
				fileTranslations[et.Text] = translated
			}
		}

		// Reconstruct the file.
		entry := pr.Input
		reconstructed, err := entry.Parser.Reconstruct(pr.Result, fileTranslations)
		if err != nil {
			log.Error().Err(err).Str("file", entry.Path).Msg("Reconstruct failed")
			continue
		}

		// Compute output path.
		relPath, err := filepath.Rel(inputAbs, entry.Path)
		if err != nil {
			log.Error().Err(err).Msg("Compute relative path")
			continue
		}
		outPath := filepath.Join(outputAbs, relPath)

		// Create parent directories.
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			log.Error().Err(err).Str("path", outPath).Msg("Create output directory")
			continue
		}

		// Write translated file.
		if err := os.WriteFile(outPath, reconstructed, 0644); err != nil {
			log.Error().Err(err).Str("path", outPath).Msg("Write output file")
			continue
		}

		log.Info().
			Str("input", entry.Path).
			Str("output", outPath).
			Int("translations", len(fileTranslations)).
			Msg("File translated")
	}

	log.Info().
		Int("files", len(entries)).
		Str("output", outputDir).
		Msg("Translation pipeline complete")

	return nil
}
