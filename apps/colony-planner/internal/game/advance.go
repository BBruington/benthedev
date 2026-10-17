package game

import (
	"fmt"
	"maps"
)

// MaxUnfoldedArrivals is the most SHIPMENT_ARRIVED entries one advance writes one by one; past it the
// arrivals fold into one entry per run, so the writes stay bounded for any N (spec §5.2).
const MaxUnfoldedArrivals = 12

// Step is one stage of a colony day. Steps run in DailySteps order on every day, and that order is a
// rule: on any given day upkeep runs before shipments.
type Step interface {
	// Span applies this step to every day in (from, to], given that no other step changes anything
	// this step reads inside that span. state.Day is from while it runs.
	Span(state *State, from int64, to int64, log *Log)
	// NextBoundary is the next day after `after` on which this step changes something another step
	// reads. Advance runs every boundary day in a span of its own, so a step that changes such state
	// every day reports after+1 and the walk becomes a day-by-day loop.
	NextBoundary(state State, after int64) (day int64, ok bool)
}

// DailySteps is the pipeline, in order. A future step is appended only if its rules need one.
var DailySteps = []Step{Upkeep{}, Shipments{}}

// ArrivalRun is the arithmetic run of shipment arrival days First, First+Every, … (Count of them).
// Its JSON is the `data` of a folded SHIPMENT_ARRIVED entry: {first, every, count}.
type ArrivalRun struct {
	First int64 `json:"first"`
	Every int64 `json:"every"`
	Count int64 `json:"count"`
}

// Last is the final arrival day of the run.
func (run ArrivalRun) Last() int64 {
	return run.First + (run.Count-1)*run.Every
}

// Log collects what the steps did across every span of one advance.
type Log struct {
	resourceDeltas map[string]int64
	arrivalRuns    []ArrivalRun
}

// RecordResourceDelta adds an applied change to a resource.
func (log *Log) RecordResourceDelta(key string, delta int64) {
	if delta == 0 {
		return
	}
	if log.resourceDeltas == nil {
		log.resourceDeltas = map[string]int64{}
	}
	log.resourceDeltas[key] += delta
	if log.resourceDeltas[key] == 0 {
		delete(log.resourceDeltas, key)
	}
}

// RecordArrivals adds shipment arrivals, extending the previous run when it continues it, so one
// cadence split across spans still folds into one {first, every, count}.
func (log *Log) RecordArrivals(run ArrivalRun) {
	if run.Count <= 0 {
		return
	}
	if last := len(log.arrivalRuns) - 1; last >= 0 {
		previous := log.arrivalRuns[last]
		if previous.Every == run.Every && previous.Last()+previous.Every == run.First {
			log.arrivalRuns[last].Count += run.Count
			return
		}
	}
	log.arrivalRuns = append(log.arrivalRuns, run)
}

// Upkeep eats FoodPerColonistPerDay food per colonist per day, floored at food's minimum. Nothing
// else happens at the floor (spec §5.2, decisions 4 and 5).
type Upkeep struct{}

func (upkeep Upkeep) Span(state *State, from int64, to int64, log *Log) {
	food, found := state.Resources[FoodResourceKey]
	if !found {
		return
	}
	eaters := Colonists(state.Roles) * FoodPerColonistPerDay
	days := to - from
	spare := food.Amount - food.MinAmount
	if eaters <= 0 || days <= 0 || spare <= 0 {
		return
	}
	newAmount := food.MinAmount
	// Dividing first keeps eaters × days from overflowing int64 for any N.
	if days <= spare/eaters {
		newAmount = food.Amount - eaters*days
	}
	log.RecordResourceDelta(FoodResourceKey, newAmount-food.Amount)
	food.Amount = newAmount
	state.Resources[FoodResourceKey] = food
}

// Colonists are constant during an advance, so upkeep changes nothing another step reads.
func (upkeep Upkeep) NextBoundary(State, int64) (int64, bool) {
	return 0, false
}

// Shipments counts the arrivals in (from, to] as an arithmetic run and moves the next arrival to the
// first day after `to`. Shipments carry nothing automatically (spec §5.3).
type Shipments struct{}

func (shipments Shipments) Span(state *State, from int64, to int64, log *Log) {
	schedule := state.Schedule
	if schedule == nil || schedule.NextDay > to || schedule.NextDay <= from {
		return
	}
	count := (to-schedule.NextDay)/schedule.IntervalDays + 1
	log.RecordArrivals(ArrivalRun{First: schedule.NextDay, Every: schedule.IntervalDays, Count: count})
	state.Schedule = &Schedule{
		IntervalDays: schedule.IntervalDays,
		NextDay:      schedule.NextDay + count*schedule.IntervalDays,
	}
}

// Arrivals deliver nothing, so they change nothing another step reads.
func (shipments Shipments) NextBoundary(State, int64) (int64, bool) {
	return 0, false
}

// ArrivalEntry is one SHIPMENT_ARRIVED timeline entry to write.
type ArrivalEntry struct {
	// Day is the entry's day: the arrival day, or the run's last arrival day when folded.
	Day int64
	// Folded marks the one entry that stands for a whole run ("142,857 shipments arrived, every 7
	// days, Days 42 – 1,000,034"); Run is then its data. Unfolded, Run is {Day, interval, 1}.
	Folded bool
	Run    ArrivalRun
}

// AdvanceResult is everything the handler needs to save an advance, or to show it as a preview.
type AdvanceResult struct {
	Days int64
	// Before is a copy of the input State, After the State to save. Before describes the undo
	// snapshot: the day, the schedule and every resource After changed.
	Before State
	After  State
	// Colonists is the headcount upkeep used.
	Colonists int64
	// Applied holds the applied resource deltas only (zero ones omitted): the DAYS_ADVANCED deltas.
	Applied Effects
	// Arrivals are every arrival in (Before.Day, After.Day], as runs; ArrivalCount is their total.
	Arrivals     []ArrivalRun
	ArrivalCount int64
	// ArrivalEntries are the SHIPMENT_ARRIVED entries to write: one per arrival day when there are
	// MaxUnfoldedArrivals or fewer, otherwise one folded entry per run.
	ArrivalEntries []ArrivalEntry
}

// Advance moves the colony forward `days` days (spec §5.2). The preview calls it too and saves nothing.
func Advance(state State, days int64) (AdvanceResult, error) {
	return advance(state, days, DailySteps)
}

func advance(state State, days int64, steps []Step) (AdvanceResult, error) {
	if err := checkStoredState(state); err != nil {
		return AdvanceResult{}, err
	}
	if _, found := state.Resources[FoodResourceKey]; !found {
		return AdvanceResult{}, fmt.Errorf("game: the colony has no %q resource", FoodResourceKey)
	}
	if days < 1 {
		return AdvanceResult{}, invalid("days", MessageAdvanceAtLeast)
	}
	if days > MaxStored-state.Day {
		return AdvanceResult{}, invalid("days", MessageTooManyDays)
	}
	target := state.Day + days
	after := state.Clone()
	var log Log
	for after.Day < target {
		from := after.Day
		to := target
		for _, step := range steps {
			boundary, ok := step.NextBoundary(after, from)
			if !ok {
				continue
			}
			// End the span the day before the boundary so the boundary day runs on its own, in step order.
			spanEnd := boundary - 1
			if spanEnd <= from {
				spanEnd = from + 1
			}
			to = min(to, spanEnd)
		}
		for _, step := range steps {
			step.Span(&after, from, to, &log)
		}
		after.Day = to
	}
	if after.Schedule != nil && after.Schedule.NextDay > MaxStored {
		return AdvanceResult{}, invalid("days", MessageTooManyDays)
	}

	result := AdvanceResult{
		Days:           days,
		Before:         state.Clone(),
		After:          after,
		Colonists:      Colonists(state.Roles),
		Applied:        Effects{Resources: maps.Clone(log.resourceDeltas)},
		Arrivals:       append([]ArrivalRun{}, log.arrivalRuns...),
		ArrivalEntries: []ArrivalEntry{},
	}
	if result.Applied.Resources == nil {
		result.Applied.Resources = map[string]int64{}
	}
	for _, run := range log.arrivalRuns {
		result.ArrivalCount += run.Count
	}
	for _, run := range log.arrivalRuns {
		if result.ArrivalCount > MaxUnfoldedArrivals {
			result.ArrivalEntries = append(result.ArrivalEntries, ArrivalEntry{Day: run.Last(), Folded: true, Run: run})
			continue
		}
		for arrival := range run.Count {
			day := run.First + arrival*run.Every
			result.ArrivalEntries = append(result.ArrivalEntries, ArrivalEntry{
				Day: day,
				Run: ArrivalRun{First: day, Every: run.Every, Count: 1},
			})
		}
	}
	return result, nil
}
