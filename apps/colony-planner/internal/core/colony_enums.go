package core

import (
	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// ColonyIcon is the Postgres colony_icon enum: the curated lucide icons a resource or role may show.
// The web maps each value to its lucide component. Re-exported so only core reaches into store.
type ColonyIcon = store.ColonyIcon

const (
	ColonyIconAnchor         = store.ColonyIconAnchor
	ColonyIconApple          = store.ColonyIconApple
	ColonyIconAxe            = store.ColonyIconAxe
	ColonyIconBeef           = store.ColonyIconBeef
	ColonyIconBeer           = store.ColonyIconBeer
	ColonyIconBone           = store.ColonyIconBone
	ColonyIconBookOpen       = store.ColonyIconBookOpen
	ColonyIconBoxes          = store.ColonyIconBoxes
	ColonyIconCoins          = store.ColonyIconCoins
	ColonyIconCrown          = store.ColonyIconCrown
	ColonyIconDroplets       = store.ColonyIconDroplets
	ColonyIconFeather        = store.ColonyIconFeather
	ColonyIconFish           = store.ColonyIconFish
	ColonyIconFlame          = store.ColonyIconFlame
	ColonyIconFlaskConical   = store.ColonyIconFlaskConical
	ColonyIconGem            = store.ColonyIconGem
	ColonyIconHammer         = store.ColonyIconHammer
	ColonyIconHardHat        = store.ColonyIconHardHat
	ColonyIconHeart          = store.ColonyIconHeart
	ColonyIconHeartHandshake = store.ColonyIconHeartHandshake
	ColonyIconHeartPulse     = store.ColonyIconHeartPulse
	ColonyIconLeaf           = store.ColonyIconLeaf
	ColonyIconMountain       = store.ColonyIconMountain
	ColonyIconPackage        = store.ColonyIconPackage
	ColonyIconPickaxe        = store.ColonyIconPickaxe
	ColonyIconScroll         = store.ColonyIconScroll
	ColonyIconShield         = store.ColonyIconShield
	ColonyIconShip           = store.ColonyIconShip
	ColonyIconShirt          = store.ColonyIconShirt
	ColonyIconSparkles       = store.ColonyIconSparkles
	ColonyIconStar           = store.ColonyIconStar
	ColonyIconSword          = store.ColonyIconSword
	ColonyIconTent           = store.ColonyIconTent
	ColonyIconTrees          = store.ColonyIconTrees
	ColonyIconUsers          = store.ColonyIconUsers
	ColonyIconWandSparkles   = store.ColonyIconWandSparkles
	ColonyIconWheat          = store.ColonyIconWheat
	ColonyIconWrench         = store.ColonyIconWrench
)

// AllColonyIconValues lists every icon, for the API's enum schema.
func AllColonyIconValues() []ColonyIcon {
	return store.AllColonyIconValues()
}

// TimelineVisibility is the Postgres timeline_visibility enum. Players see only PUBLIC entries that
// have not been undone (spec §2.2, item 3).
type TimelineVisibility = store.TimelineVisibility

const (
	TimelineVisibilityPublic = store.TimelineVisibilityPUBLIC
	TimelineVisibilityDm     = store.TimelineVisibilityDM
)

// AllTimelineVisibilityValues lists both visibilities, for the API's enum schema.
func AllTimelineVisibilityValues() []TimelineVisibility {
	return store.AllTimelineVisibilityValues()
}

// TimelineKind is what wrote a timeline entry. The column is text and this list is the one place a
// kind is written (spec §4: an open-ended kind needs no migration). Later milestones append theirs
// (FAVOR_ADJUSTED, WORKFORCE_CHANGED, QUEST_*, PATRON_ACTION, SESSION_PLAYED, ...).
type TimelineKind string

const (
	TimelineKindDaysAdvanced            TimelineKind = "DAYS_ADVANCED"
	TimelineKindShipmentArrived         TimelineKind = "SHIPMENT_ARRIVED"
	TimelineKindShipmentScheduleChanged TimelineKind = "SHIPMENT_SCHEDULE_CHANGED"
	TimelineKindCargoRecorded           TimelineKind = "CARGO_RECORDED"
	TimelineKindStockAdjusted           TimelineKind = "STOCK_ADJUSTED"
	TimelineKindDayCorrected            TimelineKind = "DAY_CORRECTED"
	TimelineKindUndo                    TimelineKind = "UNDO"
	TimelineKindCustom                  TimelineKind = "CUSTOM"
)

// AllTimelineKinds lists every kind, for the API's enum schema and the DM's filter chips.
func AllTimelineKinds() []TimelineKind {
	return []TimelineKind{
		TimelineKindDaysAdvanced,
		TimelineKindShipmentArrived,
		TimelineKindShipmentScheduleChanged,
		TimelineKindCargoRecorded,
		TimelineKindStockAdjusted,
		TimelineKindDayCorrected,
		TimelineKindUndo,
		TimelineKindCustom,
	}
}

// AdjustSource says why the DM is changing stock: a correction (STOCK_ADJUSTED, DM-only by default)
// or a shipment's cargo (CARGO_RECORDED, linked to its arrival, public by default) (spec §5.3, §5.4).
type AdjustSource string

const (
	AdjustSourceAdjustment AdjustSource = "ADJUSTMENT"
	AdjustSourceShipment   AdjustSource = "SHIPMENT"
)

// AllAdjustSources lists every source, for the API's enum schema.
func AllAdjustSources() []AdjustSource {
	return []AdjustSource{AdjustSourceAdjustment, AdjustSourceShipment}
}

// Effects is the one Effects JSON shape (spec §4.3): {"resources":{"food":7},"colonists":2}. Timeline
// deltas use it today; quest rewards and patron actions reuse it from M3.
type Effects = game.Effects
