package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// dataPackagePrefixes are the packages only core may hand around. depguard stops other packages from
// importing them, but Go infers types, so an exported core function returning *store.Queries or a
// pgx pool would let any caller reach the database without an import. This test closes that hole
// (CLAUDE.md "Boundaries").
var dataPackagePrefixes = []string{
	"github.com/BBruington/colony-planner/internal/store",
	"github.com/jackc/pgx",
	"database/sql",
}

// allowedDataReferences are the deliberate exceptions, named as they appear in core's API.
var allowedDataReferences = map[string]string{
	"Options.Pool":                   "cmd/colony hands core its database pool at startup",
	"MemberStatus":                   "the member_status enum, re-exported so callers never import store",
	"QuestKind":                      "the quest_kind enum, re-exported so callers never import store",
	"QuestKindMainland":              "a quest_kind value",
	"QuestKindColony":                "a quest_kind value",
	"QuestStatus":                    "the quest_status enum, re-exported so callers never import store",
	"QuestStatusActive":              "a quest_status value",
	"QuestStatusCompleted":           "a quest_status value",
	"QuestStatusFailed":              "a quest_status value",
	"RequestStatus":                  "the request_status enum, re-exported so callers never import store",
	"RequestStatusPending":           "a request_status value",
	"RequestStatusApproved":          "a request_status value",
	"RequestStatusDeclined":          "a request_status value",
	"MarketOrderStatus":              "the market_order_status enum, re-exported so callers never import store",
	"MarketOrderStatusUnpaid":        "a market_order_status value",
	"MarketOrderStatusPaid":          "a market_order_status value",
	"MarketOrderStatusCancelled":     "a market_order_status value",
	"RequestStatusWithdrawn":         "a request_status value",
	"MemberStatusPending":            "a member_status value",
	"MemberStatusActive":             "a member_status value",
	"MemberStatusRemoved":            "a member_status value",
	"ColonyIcon":                     "the colony_icon enum, re-exported so callers never import store",
	"ColonyIconAnchor":               "a colony_icon value",
	"ColonyIconApple":                "a colony_icon value",
	"ColonyIconAxe":                  "a colony_icon value",
	"ColonyIconBeef":                 "a colony_icon value",
	"ColonyIconBeer":                 "a colony_icon value",
	"ColonyIconBone":                 "a colony_icon value",
	"ColonyIconBookOpen":             "a colony_icon value",
	"ColonyIconBoxes":                "a colony_icon value",
	"ColonyIconCoins":                "a colony_icon value",
	"ColonyIconCrown":                "a colony_icon value",
	"ColonyIconDroplets":             "a colony_icon value",
	"ColonyIconFeather":              "a colony_icon value",
	"ColonyIconFish":                 "a colony_icon value",
	"ColonyIconFlame":                "a colony_icon value",
	"ColonyIconFlaskConical":         "a colony_icon value",
	"ColonyIconGem":                  "a colony_icon value",
	"ColonyIconHammer":               "a colony_icon value",
	"ColonyIconHardHat":              "a colony_icon value",
	"ColonyIconHeart":                "a colony_icon value",
	"ColonyIconHeartHandshake":       "a colony_icon value",
	"ColonyIconHeartPulse":           "a colony_icon value",
	"ColonyIconLeaf":                 "a colony_icon value",
	"ColonyIconMountain":             "a colony_icon value",
	"ColonyIconPackage":              "a colony_icon value",
	"ColonyIconPickaxe":              "a colony_icon value",
	"ColonyIconScroll":               "a colony_icon value",
	"ColonyIconShield":               "a colony_icon value",
	"ColonyIconShip":                 "a colony_icon value",
	"ColonyIconShirt":                "a colony_icon value",
	"ColonyIconSparkles":             "a colony_icon value",
	"ColonyIconStar":                 "a colony_icon value",
	"ColonyIconSword":                "a colony_icon value",
	"ColonyIconTent":                 "a colony_icon value",
	"ColonyIconTrees":                "a colony_icon value",
	"ColonyIconUsers":                "a colony_icon value",
	"ColonyIconWandSparkles":         "a colony_icon value",
	"ColonyIconWheat":                "a colony_icon value",
	"ColonyIconWrench":               "a colony_icon value",
	"TimelineVisibility":             "the timeline_visibility enum, re-exported so callers never import store",
	"TimelineVisibilityPublic":       "a timeline_visibility value",
	"TimelineVisibilityDm":           "a timeline_visibility value",
	"CampaignRole":                   "the campaign_role enum, re-exported so callers never import store",
	"CampaignRoleDm":                 "a campaign_role value",
	"CampaignRolePlayer":             "a campaign_role value",
	"CampaignStatus":                 "the campaign_status enum, re-exported so callers never import store",
	"CampaignStatusActive":           "a campaign_status value",
	"CampaignStatusSuspended":        "a campaign_status value",
	"CampaignRequestStatus":          "the campaign_request_status enum, re-exported so callers never import store",
	"CampaignRequestStatusPending":   "a campaign_request_status value",
	"CampaignRequestStatusApproved":  "a campaign_request_status value",
	"CampaignRequestStatusDenied":    "a campaign_request_status value",
	"CampaignRequestStatusWithdrawn": "a campaign_request_status value",
	"NpcStatus":                      "the npc_status enum, re-exported so callers never import store",
	"NpcStatusAlive":                 "an npc_status value",
	"NpcStatusDead":                  "an npc_status value",
	"NpcStatusMissing":               "an npc_status value",
	"NpcStatusUnknown":               "an npc_status value",
	"NpcHealth":                      "the npc_health enum, re-exported so callers never import store",
	"NpcHealthHealthy":               "an npc_health value",
	"NpcHealthInjured":               "an npc_health value",
	"NpcHealthSick":                  "an npc_health value",
	"NpcHealthUnknown":               "an npc_health value",
	"NpcHealthDead":                  "an npc_health value",
	"NpcRelation":                    "the npc_relation enum, re-exported so callers never import store",
	"NpcRelationAlly":                "an npc_relation value",
	"NpcRelationNeutral":             "an npc_relation value",
	"NpcRelationSuspicious":          "an npc_relation value",
	"NpcRelationEnemy":               "an npc_relation value",
	"NpcRelationUnknown":             "an npc_relation value",
	"SessionStatus":                  "the session_status enum, re-exported so callers never import store",
	"SessionStatusScheduled":         "a session_status value",
	"SessionStatusCancelled":         "a session_status value",
	"SessionStatusPlayed":            "a session_status value",
	"Terrain":                        "the terrain enum, re-exported so callers never import store",
	"TerrainForest":                  "a terrain value",
	"TerrainPlains":                  "a terrain value",
	"TerrainWater":                   "a terrain value",
	"TerrainMountain":                "a terrain value",
	"TerrainDesert":                  "a terrain value",
	"TerrainSwamp":                   "a terrain value",
	"TerrainTundra":                  "a terrain value",
	"TerrainVolcanic":                "a terrain value",
}

func TestCoreAPIExposesNoDataTypes(t *testing.T) {
	fileSet := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		dataImportNames := dataImportNamesOf(file)
		if len(dataImportNames) == 0 {
			continue
		}
		report := func(name string, node ast.Node) {
			if _, allowed := allowedDataReferences[name]; allowed {
				return
			}
			if mentionsDataPackage(node, dataImportNames) {
				violations = append(violations, fileSet.Position(node.Pos()).String()+": "+name)
			}
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Name.IsExported() && exportedReceiver(typed) {
					report(qualifiedFunctionName(typed), typed.Type)
				}
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					checkSpecification(specification, report)
				}
			}
		}
	}
	for _, violation := range violations {
		t.Errorf("core's exported API mentions a data type (store, pgx or database/sql): %s. "+
			"Return a core type instead, or add a deliberate exception to allowedDataReferences.", violation)
	}
}

func checkSpecification(specification ast.Spec, report func(string, ast.Node)) {
	switch typed := specification.(type) {
	case *ast.TypeSpec:
		if !typed.Name.IsExported() {
			return
		}
		structType, isStruct := typed.Type.(*ast.StructType)
		if !isStruct || typed.Assign.IsValid() {
			report(typed.Name.Name, typed.Type)
			return
		}
		for _, field := range structType.Fields.List {
			for _, fieldName := range field.Names {
				if fieldName.IsExported() {
					report(typed.Name.Name+"."+fieldName.Name, field.Type)
				}
			}
			if len(field.Names) == 0 {
				report(typed.Name.Name+" (embedded)", field.Type)
			}
		}
	case *ast.ValueSpec:
		for index, valueName := range typed.Names {
			if !valueName.IsExported() {
				continue
			}
			if typed.Type != nil {
				report(valueName.Name, typed.Type)
			}
			if index < len(typed.Values) {
				report(valueName.Name, typed.Values[index])
			}
		}
	}
}

// dataImportNamesOf maps each data package's local name in this file (its alias, or the last path
// element) so selector expressions like store.Queries can be recognized.
func dataImportNamesOf(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, importSpecification := range file.Imports {
		importPath, err := strconv.Unquote(importSpecification.Path.Value)
		if err != nil || !isDataPackage(importPath) {
			continue
		}
		localName := importPath[strings.LastIndex(importPath, "/")+1:]
		if majorVersionSuffix.MatchString(localName) {
			// github.com/jackc/pgx/v5 is imported as pgx.
			trimmed := strings.TrimSuffix(importPath, "/"+localName)
			localName = trimmed[strings.LastIndex(trimmed, "/")+1:]
		}
		if importSpecification.Name != nil {
			localName = importSpecification.Name.Name
		}
		names[localName] = true
	}
	return names
}

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

func isDataPackage(importPath string) bool {
	for _, prefix := range dataPackagePrefixes {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func mentionsDataPackage(node ast.Node, dataImportNames map[string]bool) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		selector, isSelector := child.(*ast.SelectorExpr)
		if !isSelector {
			return !found
		}
		if packageName, isIdentifier := selector.X.(*ast.Ident); isIdentifier && dataImportNames[packageName.Name] {
			found = true
		}
		return !found
	})
	return found
}

// exportedReceiver is true for plain functions and for methods on exported types.
func exportedReceiver(function *ast.FuncDecl) bool {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	receiverType := function.Recv.List[0].Type
	if pointer, isPointer := receiverType.(*ast.StarExpr); isPointer {
		receiverType = pointer.X
	}
	identifier, isIdentifier := receiverType.(*ast.Ident)
	return isIdentifier && identifier.IsExported()
}

func qualifiedFunctionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiverType := function.Recv.List[0].Type
	if pointer, isPointer := receiverType.(*ast.StarExpr); isPointer {
		receiverType = pointer.X
	}
	if identifier, isIdentifier := receiverType.(*ast.Ident); isIdentifier {
		return identifier.Name + "." + function.Name.Name
	}
	return function.Name.Name
}
