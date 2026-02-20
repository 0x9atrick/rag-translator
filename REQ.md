You are a senior Golang engineer and professional game localization architect.

Your task is to build a **production-grade GraphRAG-based game localization tool** in Go.

This tool localizes a Chinese wuxia MMORPG game: **剑侠世界2 (Jianxia World 2)**.

Source language: Simplified Chinese (zh-CN)
Target language: Vietnamese (vi-VN)

The tool must translate game script files while preserving syntax, interpolation, and wuxia terminology.

==================================================
TECH STACK REQUIREMENTS
==================================================

Language: Go (latest stable version)
Architecture: Clean architecture
Concurrency: goroutines + worker pool
Vector DB: PostgreSQL with pgvector
Graph DB: Neo4j (or alternative) for knowledge graph
LLM: Anthropic Opus via HTTP API
Embedding: Anthropic embedding API

No mock code.
No pseudocode.
Produce real runnable Go code.

==================================================
CORE REQUIREMENTS
==================================================

The tool must:

1. Use **GraphRAG** (Graph-augmented Retrieval Augmented Generation)
2. Build a knowledge graph connecting terminology, skills, items, characters, locations, and game entities
3. Use pgvector to store embeddings for retrieval
4. Translate Simplified Chinese → Vietnamese
5. Preserve ALL interpolation variables
6. Preserve ALL file structure exactly
7. Translate ONLY human-readable text
8. Support large projects efficiently (>100k strings)
9. Use batching and concurrency safely
10. Provide CLI interface

==================================================
SUPPORTED FILE TYPES
==================================================

- .lua
- .ini
- .txt

Important:

Some .txt files are TAB-separated structured files.  
Delimiter: `\t`  

Must detect tab-separated format automatically.

==================================================
TAB-SEPARATED TXT PARSER REQUIREMENTS
==================================================

- Detect tab-separated format based on consistent TAB count
- Preserve column count, delimiter, row order
- Translate ONLY text columns
- Example input:

1002\treward_gold\t获得%d金币


- Output:

1002\treward_gold\tNhận được %d vàng


==================================================
LUA PARSER REQUIREMENTS
==================================================

- Extract only human-readable strings
- Example:

UI.ShowMessage("获得%d金币")


- Extracted:

{
  Text: "获得%d金币",
  Context: {
    File: "UI.lua",
    Function: "ShowMessage"
  }
}

==================================================
INTERPOLATION HANDLING (CRITICAL)
==================================================

- Preserve:

%d, %s, %f, %i, {0}, {1}, ${value}


- Convert internally for safe translation:

%d → {{var_1}}


- Example:

Input:
获得%d金币


Internal:
获得{{var_1}}金币


Translated:
Nhận được {{var_1}} vàng


Restored:
Nhận được %d vàng


- Must be deterministic and safe.

==================================================
WUXIA TERMINOLOGY RULES FOR 剑侠世界2
==================================================

- Translate using correct Vietnamese wuxia terminology
- Examples specific to 剑侠世界2:

真气 → Chân khí
内功 → Nội công
外功 → Ngoại công
轻功 → Khinh công
门派 → Môn phái
掌门 → Chưởng môn
弟子 → Đệ tử
帮派 → Bang phái
副本 → Phó bản
经验 → Kinh nghiệm
装备 → Trang bị
强化 → Cường hóa
等级 → Cấp
技能 → Kỹ năng
坐骑 → Ngựa cưỡi
心法 → Tâm pháp
心法等级 → Cấp tâm pháp
藏宝图 → Bản đồ kho báu
江湖 → Giang hồ
门派任务 → Nhiệm vụ môn phái


- Do NOT translate literally if wuxia term exists
- Knowledge graph must store entities, relationships, and terminology

==================================================
GRAPHRAG REQUIREMENTS
==================================================

- Build a knowledge graph of game entities (skills, buffs, items, characters, locations, terminology)
- Connect relationships, e.g.:

真气 → used in → 技能
技能 → belongs to → 门派
装备 → requires → 等级
心法 → improves → 技能


- Graph DB (Neo4j or alternative) to store and query relationships
- Retrieve relevant subgraph context before translation
- Combine graph retrieval with vector embedding search for best context

==================================================
RAG + GRAPH FLOW
==================================================

1. Parse files → extract text and context
2. Generate embeddings → store in pgvector
3. Build / update knowledge graph
4. On translation request:
   - Retrieve top-K relevant embeddings
   - Retrieve relevant subgraph from knowledge graph
   - Build context + terminology prompt
   - Call Opus LLM
5. Restore interpolation
6. Write output maintaining original file structure

==================================================
TRANSLATION PROMPT
==================================================

System prompt:

You are a professional Vietnamese localizer specializing in Chinese wuxia MMORPG games, specifically 剑侠世界2.

Rules:

Translate Simplified Chinese to Vietnamese

Use correct wuxia terminology from knowledge graph

Preserve placeholders like {{var_1}}

Preserve formatting and syntax

Output ONLY Vietnamese translation


User prompt template:

Context:
{{retrieved_context_from_graph_and_embedding}}

Terminology:
{{terminology_from_graph}}

Text:
{{text_to_translate}}


==================================================
ARCHITECTURE
==================================================

Project structure:

cmd/
  main.go

internal/
  cli/
  parser/
    lua.go
    ini.go
    txt.go
  rag/
    vector_store.go
    embed.go
    retrieve.go
  graph/
    graph_builder.go
    graph_query.go
  translation/
    opus_client.go
    prompt_builder.go
  interpolation/
    interpolation.go
  cache/
    cache.go
  worker/
    pool.go
  filewalker/
    walker.go

==================================================
CLI REQUIREMENTS
==================================================

Commands:

- ingest: `go run cmd/main.go ingest ./game-files`
- translate: `go run cmd/main.go translate ./game-files ./output`

==================================================
CONCURRENCY REQUIREMENTS
==================================================

- Use worker pool for file processing
- Limit concurrent API calls
- Use context.Context for cancellation
- Graceful shutdown support

==================================================
PERFORMANCE REQUIREMENTS
==================================================

- Batch embedding
- Batch translation
- Cache repeated strings
- Deduplicate by hash
- Stream file processing

==================================================
IMPLEMENTATION REQUIREMENTS
==================================================

- database/sql + pgx for PostgreSQL
- Neo4j Go driver for graph
- Structured logging
- Config via environment variables
- Clean interfaces and DI
- Proper error handling

==================================================
OUTPUT REQUIREMENTS
==================================================

- Full runnable Go project
- All modules implemented
- Parsers, GraphRAG retrieval, embeddings, Opus client, CLI
- Interpolation handler
- Caching

==================================================
QUALITY REQUIREMENTS
==================================================

- Production-grade
- Clean, well-typed
- Extensible
- Maintainable

==================================================
BEGIN IMPLEMENTATION
==================================================

Generate the complete Golang project implementing GraphRAG for **剑侠世界2** game localization.
