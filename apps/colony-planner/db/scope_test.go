package db_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/BBruington/colony-planner/db/dbtest"
)

// The SQL scope lint (docs/design/multi-campaign.md §3.3). It is a token heuristic, not a SQL
// parser: the cross-campaign secrecy sweep is the backstop for anything it misses. Every query that
// names a campaign-owned table (one with a campaign_id column) must take @campaign_id and filter each
// occurrence of such a table by campaign_id, unless the line above `-- name:` says
// `-- unscoped: <reason>`.

// directoryTables have a campaign_id column but are directory data, read without a campaign by design.
var directoryTables = map[string]string{
	"campaign_request":   "an account's request to run a campaign; campaign_id is set only once the owner approves it",
	"discord_guild_link": "Beny Bot resolves a Discord server to its campaign before it knows the campaign",
}

type queryBlock struct {
	location       string
	name           string
	unscopedReason string
	hasUnscoped    bool
	sql            string
}

var (
	queryNamePattern = regexp.MustCompile(`^--\s*name:\s*(\S+)`)
	unscopedPattern  = regexp.MustCompile(`^--\s*unscoped:\s*(.*)$`)
	tokenPattern     = regexp.MustCompile(`@[A-Za-z_][A-Za-z0-9_]*|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*|::|\S`)
	stringPattern    = regexp.MustCompile(`'(?:[^']|'')*'`)
)

// parseQueryBlocks splits a sqlc queries file into its `-- name:` blocks.
func parseQueryBlocks(fileName string, source string) []queryBlock {
	var blocks []queryBlock
	var current *queryBlock
	previousLine := ""
	for index, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if match := queryNamePattern.FindStringSubmatch(trimmed); match != nil {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &queryBlock{location: fileName + ":" + strconv.Itoa(index+1), name: match[1]}
			if unscoped := unscopedPattern.FindStringSubmatch(previousLine); unscoped != nil {
				current.hasUnscoped = true
				current.unscopedReason = strings.TrimSpace(unscoped[1])
			}
		} else if current != nil {
			code, _, _ := strings.Cut(line, "--")
			current.sql += code + "\n"
		}
		previousLine = trimmed
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	return blocks
}

// scopeProblems is the lint for one query: it names what is wrong, or nothing.
func scopeProblems(sql string, campaignOwned map[string]bool) []string {
	tokens := tokenPattern.FindAllString(stringPattern.ReplaceAllString(sql, "''"), -1)
	lower := make([]string, len(tokens))
	for index, token := range tokens {
		lower[index] = strings.ToLower(token)
	}
	isParameter := func(index int) bool {
		return index >= 2 && lower[index-1] == "(" && (lower[index-2] == "sqlc.arg" || lower[index-2] == "sqlc.narg")
	}
	isColumn := func(index int) bool {
		token := lower[index]
		return (token == "campaign_id" || strings.HasSuffix(token, ".campaign_id")) && !isParameter(index)
	}

	var problems []string
	tableOccurrences := 0
	columnMentions := 0
	hasParameter := false
	var namedTables []string
	for index, token := range lower {
		if token == "@campaign_id" || (token == "campaign_id" && isParameter(index)) {
			hasParameter = true
		}
		if isColumn(index) {
			columnMentions++
		}
		if !campaignOwned[token] || index == 0 {
			continue
		}
		switch lower[index-1] {
		case "from", "join", "update", "into":
		default:
			continue
		}
		tableOccurrences++
		namedTables = append(namedTables, token)
		if lower[index-1] != "join" {
			continue
		}
		joinFiltered := false
		for next := index + 1; next < len(lower); next++ {
			if slices.Contains([]string{"join", "left", "right", "inner", "full", "cross", "where", "order", "group", "limit", ")", ";", "for"}, lower[next]) {
				break
			}
			if isColumn(next) {
				joinFiltered = true
				break
			}
		}
		if !joinFiltered {
			problems = append(problems, "the JOIN of "+token+" doesn't match on campaign_id")
		}
	}
	if tableOccurrences == 0 {
		return problems
	}
	if !hasParameter {
		problems = append(problems, "it names "+strings.Join(namedTables, ", ")+" but takes no @campaign_id")
	}
	if columnMentions < tableOccurrences {
		problems = append(problems, "it names campaign-owned tables "+strconv.Itoa(tableOccurrences)+
			" times but filters campaign_id only "+strconv.Itoa(columnMentions)+" times")
	}
	return problems
}

// plannerReason is the one reason Beny Bot's planner may read across campaigns (design §9.3).
const plannerReason = "planner"

// plannerProblems keeps a `-- unscoped: planner` query to what the planner may read: only ACTIVE
// campaigns that are linked to a Discord server.
func plannerProblems(sql string) []string {
	normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
	var problems []string
	if !strings.Contains(normalized, "join discord_guild_link on discord_guild_link.campaign_id = campaign.id") {
		problems = append(problems, "it must join discord_guild_link on the campaign, so unlinked campaigns stay out")
	}
	if !strings.Contains(normalized, "campaign.status = 'active'") {
		problems = append(problems, "it must keep to campaign.status = 'ACTIVE', so paused campaigns stay out")
	}
	if strings.Contains(normalized, " or ") {
		problems = append(problems, "it may not use OR, which could widen either condition")
	}
	for _, outer := range []string{"left join", "right join", "full join", "outer join"} {
		if strings.Contains(normalized, outer) {
			problems = append(problems, "it may not use an outer join, which keeps unlinked campaigns")
		}
	}
	return problems
}

// campaignOwnedTables reads, from a freshly migrated schema, every table with a campaign_id column,
// so a table added later is covered without editing this test.
func campaignOwnedTables(t *testing.T) map[string]bool {
	t.Helper()
	database := dbtest.New(t)
	rows, err := database.Pool.Query(context.Background(), `
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND column_name = 'campaign_id'`)
	if err != nil {
		t.Fatalf("read the campaign-owned tables: %v", err)
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatal(err)
		}
		if _, directory := directoryTables[tableName]; !directory {
			owned[tableName] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"settings", "colony", "resource", "role", "timeline_event", "campaign_member"} {
		if !owned[expected] {
			t.Fatalf("%s should be campaign-owned; found %v", expected, owned)
		}
	}
	return owned
}

func TestEveryQueryIsCampaignScoped(t *testing.T) {
	owned := campaignOwnedTables(t)
	paths, err := filepath.Glob("queries/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	unscopedNames := []string{}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range parseQueryBlocks(path, string(source)) {
			if block.hasUnscoped {
				if block.unscopedReason == "" {
					t.Errorf("%s %s: `-- unscoped:` needs a reason", block.location, block.name)
				}
				unscopedNames = append(unscopedNames, block.name)
				if block.unscopedReason == plannerReason {
					for _, problem := range plannerProblems(block.sql) {
						t.Errorf("%s %s: %s", block.location, block.name, problem)
					}
				}
				continue
			}
			for _, problem := range scopeProblems(block.sql, owned) {
				t.Errorf("%s %s: %s (filter it by campaign_id, or say `-- unscoped: <reason>` above `-- name:`)", block.location, block.name, problem)
			}
		}
	}
	slices.Sort(unscopedNames)
	expected := []string{"CampaignByIcsToken", "CampaignByInviteCode", "ListCampaignsForOwner", "ListMembershipsForAccount", "ListPlannerCampaigns", "ViewerForGuild"}
	if !slices.Equal(unscopedNames, expected) {
		t.Errorf("unscoped queries = %v, want %v; a new one needs a reviewed reason and a line here", unscopedNames, expected)
	}
}

func TestScopeLintCatchesBadQueries(t *testing.T) {
	owned := map[string]bool{"resource": true, "role": true, "npc": true, "timeline_event": true}
	bad := map[string]string{
		"no campaign parameter": `SELECT * FROM resource WHERE key = @key;`,
		"UPDATE ... FROM filters one table": `UPDATE resource SET amount = @amount FROM role
			WHERE resource.campaign_id = @campaign_id AND role.key = resource.key;`,
		"subquery filters only the outer table": `SELECT * FROM role
			WHERE campaign_id = @campaign_id AND key IN (SELECT key FROM resource);`,
		"colonist count filters role but not npc": `SELECT (SELECT sum(unnamed_count) FROM role WHERE campaign_id = @campaign_id)
			+ (SELECT count(*) FROM npc WHERE colony_role_key IS NOT NULL);`,
		"JOIN without campaign_id": `SELECT * FROM role JOIN resource ON resource.key = role.key
			WHERE role.campaign_id = @campaign_id AND resource.campaign_id = @campaign_id;`,
		"only the parameter, no column":         `DELETE FROM timeline_event WHERE id = @id AND @campaign_id::uuid IS NOT NULL;`,
		"sqlc.arg is a parameter, not a filter": `SELECT * FROM resource WHERE key = sqlc.arg(campaign_id);`,
	}
	for name, sql := range bad {
		if problems := scopeProblems(sql, owned); len(problems) == 0 {
			t.Errorf("%s: the lint accepted %q", name, sql)
		}
	}
	good := map[string]string{
		"select":     `SELECT * FROM resource WHERE campaign_id = @campaign_id ORDER BY sort_order;`,
		"update":     `UPDATE resource SET amount = @amount WHERE campaign_id = @campaign_id AND key = @key;`,
		"delete":     `DELETE FROM timeline_event WHERE campaign_id = @campaign_id AND id = @id;`,
		"insert":     `INSERT INTO resource (campaign_id, key) VALUES (@campaign_id, @key);`,
		"sqlc.arg":   `SELECT * FROM role WHERE campaign_id = sqlc.arg(campaign_id);`,
		"join":       `SELECT * FROM role JOIN resource ON resource.campaign_id = role.campaign_id AND resource.key = role.key WHERE role.campaign_id = @campaign_id;`,
		"subquery":   `SELECT * FROM role WHERE campaign_id = @campaign_id AND key IN (SELECT key FROM resource WHERE resource.campaign_id = @campaign_id);`,
		"two counts": `SELECT (SELECT sum(unnamed_count) FROM role WHERE campaign_id = @campaign_id) + (SELECT count(*) FROM npc WHERE campaign_id = @campaign_id);`,
		"directory":  `SELECT * FROM account WHERE id = @id;`,
		"string":     `SELECT * FROM resource WHERE campaign_id = @campaign_id AND label = 'FROM role';`,
	}
	for name, sql := range good {
		if problems := scopeProblems(sql, owned); len(problems) > 0 {
			t.Errorf("%s: the lint refused %q: %v", name, sql, problems)
		}
	}
	plannerGood := `SELECT campaign.id FROM campaign
		JOIN discord_guild_link ON discord_guild_link.campaign_id = campaign.id
		WHERE campaign.status = 'ACTIVE';`
	if problems := plannerProblems(plannerGood); len(problems) > 0 {
		t.Errorf("the planner lint refused %q: %v", plannerGood, problems)
	}
	for name, sql := range map[string]string{
		"every campaign":        `SELECT campaign.id FROM campaign WHERE campaign.status = 'ACTIVE';`,
		"suspended ones too":    `SELECT campaign.id FROM campaign JOIN discord_guild_link ON discord_guild_link.campaign_id = campaign.id;`,
		"widened with OR":       `SELECT campaign.id FROM campaign JOIN discord_guild_link ON discord_guild_link.campaign_id = campaign.id WHERE campaign.status = 'ACTIVE' OR true;`,
		"a left join keeps all": `SELECT campaign.id FROM campaign LEFT JOIN discord_guild_link ON discord_guild_link.campaign_id = campaign.id WHERE campaign.status = 'ACTIVE';`,
	} {
		if problems := plannerProblems(sql); len(problems) == 0 {
			t.Errorf("%s: the planner lint accepted %q", name, sql)
		}
	}
	blocks := parseQueryBlocks("example.sql", "-- why\n-- unscoped: the owner's counts\n-- name: Counts :many\nSELECT * FROM role;\n\n-- name: Scoped :one\nSELECT 1;\n")
	if len(blocks) != 2 || !blocks[0].hasUnscoped || blocks[0].unscopedReason != "the owner's counts" || blocks[1].hasUnscoped {
		t.Fatalf("blocks = %+v", blocks)
	}
}
