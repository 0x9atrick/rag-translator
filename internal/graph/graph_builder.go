package graph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rs/zerolog/log"
)

// WuxiaTerm represents a Chinese→Vietnamese terminology mapping with metadata.
type WuxiaTerm struct {
	Chinese    string
	Vietnamese string
	Category   string // skill, item, character, location, faction, general
}

// Relationship represents a directed edge in the knowledge graph.
type Relationship struct {
	FromChinese string
	FromType    string
	RelType     string
	ToChinese   string
	ToType      string
}

// GraphBuilder seeds and updates the Neo4j knowledge graph.
type GraphBuilder struct {
	driver neo4j.DriverWithContext
}

// NewGraphBuilder creates a new graph builder.
func NewGraphBuilder(driver neo4j.DriverWithContext) *GraphBuilder {
	return &GraphBuilder{driver: driver}
}

// EnsureSchema creates constraints and indexes on the Neo4j database.
func (gb *GraphBuilder) EnsureSchema(ctx context.Context) error {
	session := gb.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	constraints := []string{
		"CREATE CONSTRAINT IF NOT EXISTS FOR (t:Term) REQUIRE t.chinese IS UNIQUE",
	}

	for _, c := range constraints {
		if _, err := session.Run(ctx, c, nil); err != nil {
			return fmt.Errorf("create constraint: %w", err)
		}
	}

	log.Info().Msg("Graph schema ensured")
	return nil
}

// SeedTerminology populates the knowledge graph with wuxia terminology for 剑侠世界2.
func (gb *GraphBuilder) SeedTerminology(ctx context.Context) error {
	terms := getJianxiaTerminology()
	relationships := getJianxiaRelationships()

	session := gb.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	// Upsert terms.
	for _, t := range terms {
		_, err := session.Run(ctx, `
			MERGE (t:Term {chinese: $chinese})
			SET t.vietnamese = $vietnamese,
			    t.category = $category
		`, map[string]any{
			"chinese":    t.Chinese,
			"vietnamese": t.Vietnamese,
			"category":   t.Category,
		})
		if err != nil {
			return fmt.Errorf("upsert term %s: %w", t.Chinese, err)
		}
	}

	log.Info().Int("terms", len(terms)).Msg("Seeded terminology nodes")

	// Create relationships.
	for _, r := range relationships {
		_, err := session.Run(ctx, fmt.Sprintf(`
			MATCH (a:Term {chinese: $from})
			MATCH (b:Term {chinese: $to})
			MERGE (a)-[:%s]->(b)
		`, r.RelType), map[string]any{
			"from": r.FromChinese,
			"to":   r.ToChinese,
		})
		if err != nil {
			log.Warn().Err(err).
				Str("from", r.FromChinese).
				Str("to", r.ToChinese).
				Str("rel", r.RelType).
				Msg("Failed to create relationship")
		}
	}

	log.Info().Int("relationships", len(relationships)).Msg("Seeded terminology relationships")
	return nil
}

// AddEntityFromText extracts and stores game entities found in parsed text.
func (gb *GraphBuilder) AddEntityFromText(ctx context.Context, text, filePath, context string) error {
	session := gb.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	// Store the text as a TextNode for reference.
	_, err := session.Run(ctx, `
		MERGE (t:TextNode {text: $text})
		SET t.file = $file, t.context = $context
	`, map[string]any{
		"text":    text,
		"file":    filePath,
		"context": context,
	})
	if err != nil {
		return fmt.Errorf("add text node: %w", err)
	}

	// Link text to any matching terms.
	_, err = session.Run(ctx, `
		MATCH (term:Term)
		WHERE $text CONTAINS term.chinese
		MATCH (t:TextNode {text: $text})
		MERGE (t)-[:CONTAINS_TERM]->(term)
	`, map[string]any{
		"text": text,
	})
	if err != nil {
		return fmt.Errorf("link text to terms: %w", err)
	}

	return nil
}

// getJianxiaTerminology returns the complete terminology for 剑侠世界2.
func getJianxiaTerminology() []WuxiaTerm {
	return []WuxiaTerm{
		// Core combat / cultivation
		{Chinese: "真气", Vietnamese: "Chân khí", Category: "cultivation"},
		{Chinese: "内功", Vietnamese: "Nội công", Category: "cultivation"},
		{Chinese: "外功", Vietnamese: "Ngoại công", Category: "cultivation"},
		{Chinese: "轻功", Vietnamese: "Khinh công", Category: "cultivation"},
		{Chinese: "心法", Vietnamese: "Tâm pháp", Category: "cultivation"},
		{Chinese: "心法等级", Vietnamese: "Cấp tâm pháp", Category: "cultivation"},

		// Organization / social
		{Chinese: "门派", Vietnamese: "Môn phái", Category: "faction"},
		{Chinese: "掌门", Vietnamese: "Chưởng môn", Category: "character"},
		{Chinese: "弟子", Vietnamese: "Đệ tử", Category: "character"},
		{Chinese: "帮派", Vietnamese: "Bang phái", Category: "faction"},

		// Game mechanics
		{Chinese: "副本", Vietnamese: "Phó bản", Category: "gameplay"},
		{Chinese: "经验", Vietnamese: "Kinh nghiệm", Category: "gameplay"},
		{Chinese: "装备", Vietnamese: "Trang bị", Category: "item"},
		{Chinese: "强化", Vietnamese: "Cường hóa", Category: "gameplay"},
		{Chinese: "等级", Vietnamese: "Cấp", Category: "gameplay"},
		{Chinese: "技能", Vietnamese: "Kỹ năng", Category: "skill"},
		{Chinese: "坐骑", Vietnamese: "Ngựa cưỡi", Category: "item"},

		// Exploration
		{Chinese: "藏宝图", Vietnamese: "Bản đồ kho báu", Category: "item"},
		{Chinese: "江湖", Vietnamese: "Giang hồ", Category: "location"},
		{Chinese: "门派任务", Vietnamese: "Nhiệm vụ môn phái", Category: "gameplay"},

		// Additional common terms
		{Chinese: "侠客", Vietnamese: "Hiệp khách", Category: "character"},
		{Chinese: "武功", Vietnamese: "Võ công", Category: "combat"},
		{Chinese: "秘籍", Vietnamese: "Bí kíp", Category: "item"},
		{Chinese: "丹药", Vietnamese: "Đan dược", Category: "item"},
		{Chinese: "暗器", Vietnamese: "Ám khí", Category: "item"},
		{Chinese: "阵法", Vietnamese: "Trận pháp", Category: "skill"},
		{Chinese: "气血", Vietnamese: "Khí huyết", Category: "cultivation"},
		{Chinese: "穴位", Vietnamese: "Huyệt vị", Category: "cultivation"},
		{Chinese: "经脉", Vietnamese: "Kinh mạch", Category: "cultivation"},
		{Chinese: "境界", Vietnamese: "Cảnh giới", Category: "cultivation"},
		{Chinese: "修炼", Vietnamese: "Tu luyện", Category: "cultivation"},
		{Chinese: "突破", Vietnamese: "Đột phá", Category: "cultivation"},
		{Chinese: "宝石", Vietnamese: "Bảo thạch", Category: "item"},
		{Chinese: "锻造", Vietnamese: "Đúc rèn", Category: "gameplay"},
		{Chinese: "任务", Vietnamese: "Nhiệm vụ", Category: "gameplay"},
		{Chinese: "背包", Vietnamese: "Ba lô", Category: "gameplay"},
		{Chinese: "商城", Vietnamese: "Thương thành", Category: "gameplay"},
		{Chinese: "金币", Vietnamese: "Vàng", Category: "currency"},
		{Chinese: "元宝", Vietnamese: "Nguyên bảo", Category: "currency"},
		{Chinese: "银两", Vietnamese: "Bạc", Category: "currency"},
		{Chinese: "攻击", Vietnamese: "Tấn công", Category: "combat"},
		{Chinese: "防御", Vietnamese: "Phòng ngự", Category: "combat"},
		{Chinese: "暴击", Vietnamese: "Bạo kích", Category: "combat"},
		{Chinese: "闪避", Vietnamese: "Né tránh", Category: "combat"},
		{Chinese: "命中", Vietnamese: "Mệnh trúng", Category: "combat"},
		{Chinese: "生命", Vietnamese: "Sinh mệnh", Category: "combat"},
		{Chinese: "法力", Vietnamese: "Pháp lực", Category: "combat"},
	}
}

// getJianxiaRelationships returns relationships between game entities.
func getJianxiaRelationships() []Relationship {
	return []Relationship{
		{FromChinese: "真气", RelType: "USED_IN", ToChinese: "技能"},
		{FromChinese: "技能", RelType: "BELONGS_TO", ToChinese: "门派"},
		{FromChinese: "装备", RelType: "REQUIRES", ToChinese: "等级"},
		{FromChinese: "心法", RelType: "IMPROVES", ToChinese: "技能"},
		{FromChinese: "内功", RelType: "TYPE_OF", ToChinese: "武功"},
		{FromChinese: "外功", RelType: "TYPE_OF", ToChinese: "武功"},
		{FromChinese: "轻功", RelType: "TYPE_OF", ToChinese: "武功"},
		{FromChinese: "掌门", RelType: "LEADS", ToChinese: "门派"},
		{FromChinese: "弟子", RelType: "MEMBER_OF", ToChinese: "门派"},
		{FromChinese: "门派任务", RelType: "ASSIGNED_BY", ToChinese: "门派"},
		{FromChinese: "强化", RelType: "APPLIED_TO", ToChinese: "装备"},
		{FromChinese: "宝石", RelType: "ENHANCES", ToChinese: "装备"},
		{FromChinese: "经脉", RelType: "CHANNELS", ToChinese: "真气"},
		{FromChinese: "修炼", RelType: "INCREASES", ToChinese: "境界"},
		{FromChinese: "突破", RelType: "ADVANCES", ToChinese: "境界"},
		{FromChinese: "丹药", RelType: "RESTORES", ToChinese: "气血"},
		{FromChinese: "秘籍", RelType: "TEACHES", ToChinese: "技能"},
		{FromChinese: "暗器", RelType: "TYPE_OF", ToChinese: "装备"},
		{FromChinese: "阵法", RelType: "TYPE_OF", ToChinese: "技能"},
		{FromChinese: "锻造", RelType: "CREATES", ToChinese: "装备"},
		{FromChinese: "副本", RelType: "REWARDS", ToChinese: "经验"},
		{FromChinese: "副本", RelType: "DROPS", ToChinese: "装备"},
	}
}
