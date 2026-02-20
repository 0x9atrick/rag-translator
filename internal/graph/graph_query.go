package graph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rs/zerolog/log"
)

// TermResult represents a terminology match from the graph.
type TermResult struct {
	Chinese    string
	Vietnamese string
	Category   string
}

// RelationshipResult represents a graph relationship.
type RelationshipResult struct {
	From string
	Type string
	To   string
}

// QueryResult holds the combined results from a graph query.
type QueryResult struct {
	Terms         []TermResult
	Relationships []RelationshipResult
}

// GraphQuerier queries the Neo4j knowledge graph for translation context.
type GraphQuerier struct {
	driver neo4j.DriverWithContext
}

// NewGraphQuerier creates a new graph querier.
func NewGraphQuerier(driver neo4j.DriverWithContext) *GraphQuerier {
	return &GraphQuerier{driver: driver}
}

// FindRelatedTerms finds all terminology and relationships relevant to the given text.
func (gq *GraphQuerier) FindRelatedTerms(ctx context.Context, text string) (*QueryResult, error) {
	session := gq.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result := &QueryResult{}

	// Find terms whose Chinese text appears in the input.
	termsResult, err := session.Run(ctx, `
		MATCH (t:Term)
		WHERE $text CONTAINS t.chinese
		RETURN t.chinese AS chinese, t.vietnamese AS vietnamese, t.category AS category
		ORDER BY size(t.chinese) DESC
	`, map[string]any{"text": text})
	if err != nil {
		return nil, fmt.Errorf("query terms: %w", err)
	}

	for termsResult.Next(ctx) {
		record := termsResult.Record()
		chinese, _ := record.Get("chinese")
		vietnamese, _ := record.Get("vietnamese")
		category, _ := record.Get("category")

		result.Terms = append(result.Terms, TermResult{
			Chinese:    fmt.Sprintf("%v", chinese),
			Vietnamese: fmt.Sprintf("%v", vietnamese),
			Category:   fmt.Sprintf("%v", category),
		})
	}

	if len(result.Terms) == 0 {
		return result, nil
	}

	// Find 1-hop relationships for matched terms.
	relsResult, err := session.Run(ctx, `
		MATCH (t:Term)
		WHERE $text CONTAINS t.chinese
		MATCH (t)-[r]->(neighbor:Term)
		RETURN t.chinese AS from_node, type(r) AS rel_type, neighbor.chinese AS to_node
		UNION
		MATCH (t:Term)
		WHERE $text CONTAINS t.chinese
		MATCH (neighbor:Term)-[r]->(t)
		RETURN neighbor.chinese AS from_node, type(r) AS rel_type, t.chinese AS to_node
	`, map[string]any{"text": text})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to query relationships")
		return result, nil
	}

	for relsResult.Next(ctx) {
		record := relsResult.Record()
		from, _ := record.Get("from_node")
		relType, _ := record.Get("rel_type")
		to, _ := record.Get("to_node")

		result.Relationships = append(result.Relationships, RelationshipResult{
			From: fmt.Sprintf("%v", from),
			Type: fmt.Sprintf("%v", relType),
			To:   fmt.Sprintf("%v", to),
		})
	}

	log.Debug().
		Int("terms", len(result.Terms)).
		Int("relationships", len(result.Relationships)).
		Msg("Graph query complete")

	return result, nil
}

// GetAllTerminology retrieves all terminology from the graph as a lookup map.
func (gq *GraphQuerier) GetAllTerminology(ctx context.Context) (map[string]string, error) {
	session := gq.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (t:Term)
		RETURN t.chinese AS chinese, t.vietnamese AS vietnamese
	`, nil)
	if err != nil {
		return nil, fmt.Errorf("get all terminology: %w", err)
	}

	terms := make(map[string]string)
	for result.Next(ctx) {
		record := result.Record()
		chinese, _ := record.Get("chinese")
		vietnamese, _ := record.Get("vietnamese")
		terms[fmt.Sprintf("%v", chinese)] = fmt.Sprintf("%v", vietnamese)
	}

	log.Info().Int("count", len(terms)).Msg("Loaded terminology from graph")
	return terms, nil
}
