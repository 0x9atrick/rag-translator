package translation

import (
	"fmt"
	"strings"

	"rag-translator/internal/rag"
)

// PromptBuilder constructs system and user prompts for translation.
type PromptBuilder struct{}

// NewPromptBuilder creates a new prompt builder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

const systemPrompt = `You are a professional Vietnamese localizer specializing in Chinese wuxia MMORPG games, specifically 剑侠世界2 (Jianxia World 2).

Rules:
1. Translate Simplified Chinese to Vietnamese.
2. Use correct wuxia terminology from the provided knowledge graph context.
3. Preserve ALL placeholders like {{var_1}}, {{var_2}}, etc. — copy them exactly as-is into your translation.
4. Preserve ALL formatting, syntax, and special characters.
5. Output ONLY the Vietnamese translation, nothing else.
6. Do NOT add explanations, notes, or extra text.
7. If a term has a standard wuxia Vietnamese translation, always use it.
8. Maintain the same tone and register as the original.
9. For game UI text, keep it concise and natural in Vietnamese.`

// GetSystemPrompt returns the system prompt for translation.
func (pb *PromptBuilder) GetSystemPrompt() string {
	return systemPrompt
}

// BuildUserPrompt constructs the user prompt with RAG context.
func (pb *PromptBuilder) BuildUserPrompt(text string, retriever *rag.Retriever, retrievalResult *rag.RetrievalResult) string {
	var sb strings.Builder

	// Add retrieval context if available.
	if retrievalResult != nil {
		contextStr := retriever.BuildContextString(retrievalResult)
		if contextStr != "" {
			sb.WriteString(contextStr)
		}
	}

	sb.WriteString(fmt.Sprintf("Text to translate:\n%s", text))

	return sb.String()
}

// BuildBatchUserPrompt constructs a prompt for batch translations.
func (pb *PromptBuilder) BuildBatchUserPrompt(texts []string, terminologyMap map[string]string) string {
	var sb strings.Builder

	// Add terminology context.
	if len(terminologyMap) > 0 {
		sb.WriteString("=== Terminology Reference ===\n")
		for zh, vi := range terminologyMap {
			sb.WriteString(fmt.Sprintf("• %s → %s\n", zh, vi))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Translate each text below. Return ONLY the translations, separated by ||| delimiter, in the same order.\n\n")
	for i, t := range texts {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, t))
	}

	return sb.String()
}
