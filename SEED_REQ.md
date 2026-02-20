You are a senior Golang engineer and professional game localization architect.

Your task is to extend the existing Golang GraphRAG-based game localization tool for **剑侠世界2** by adding a **seed ingestion CLI command**.

The goal is to automatically generate a seed corpus for RAG from **manual translations already committed in Git**, and ingest them into the existing GraphRAG system.

==================================================
CORE REQUIREMENTS
==================================================

The new feature must:

1. Provide a CLI command: `ingest-seed-git <commit_base> <commit_target> <folder>`  

   - `commit_base`: commit hash or branch before translations
   - `commit_target`: commit hash or branch with translated lines
   - `folder`: folder of scripts to compare

2. For each file in `<folder>` (.lua, .ini, .txt):

   - Use `git diff <commit_base> <commit_target> -- <file>` to detect changes
   - Detect added translations (`+` lines) vs original (`-` lines)
   - Extract **source_text → translated_text pairs**
   - Preserve **context**:
     - file path
     - Lua function if applicable
     - tab-separated key if .txt file
     - entity_type if detectable (skill, buff, item, etc.)
   - Preserve **interpolation placeholders** (`%d`, `%s`, `{0}`, etc.)

3. Generate **seed corpus file** in TSV or JSON:

source_text\ttranslated_text\tfile\tfunction\tentity_type
获得真气\tNhận được Chân khí\tbuff.lua\tOnGain\tbuff
技能升级\tKỹ năng nâng cấp\tskill.lua\tUpgrade\tskill


4. Ingest extracted seed entries into **GraphRAG system**:

   - Compute embeddings (pgvector)
   - Add/update nodes in knowledge graph (Neo4j or equivalent)
   - Ensure seed entries are **prioritized during translation retrieval**

5. Support incremental updates:

   - If Git diff shows new translations later, ingest automatically without duplicating previous entries
   - Use **hash of source_text** to deduplicate

==================================================
ARCHITECTURE EXTENSIONS
==================================================

Add new Golang modules under `internal/seed/`:

- `git_ingest.go` → handle Git diff, extract source → translated pairs
- `vector_seed.go` → compute embeddings and insert into pgvector
- `graph_seed.go` → create/update graph nodes for seed entries

Update `cmd/main.go`:

- Add new command `ingest-seed-git` calling the above modules
- Ensure proper CLI flags, logging, and error handling

Integrate with existing GraphRAG translation flow:

1. Retrieve seed entries first
2. Retrieve general RAG embeddings + graph
3. Combine for Opus prompt
4. Translate new files consistently with manual translations

==================================================
CONCURRENCY & PERFORMANCE
==================================================

- Support parallel file processing
- Use worker pool for efficiency
- Limit concurrent LLM API calls
- Graceful cancellation using `context.Context`

==================================================
OUTPUT REQUIREMENTS
==================================================

- Full runnable Go module for seed ingestion from Git
- Produce **TSV/JSON seed corpus**
- Integrate automatically into existing GraphRAG pipeline
- Include logging and proper error handling
- Preserve interpolation and context

==================================================
BEGIN IMPLEMENTATION
==================================================

Generate the complete Go code for:

- CLI `ingest-seed-git`
- Git diff parsing
- Seed extraction
- Embedding generation
- Graph node creation/update
- Integration with existing GraphRAG translation flow