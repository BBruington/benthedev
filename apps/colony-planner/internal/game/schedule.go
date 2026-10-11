package game

import "strconv"

// ScheduleInput sets the shipment cadence (spec §5.3).
type ScheduleInput struct {
	IntervalDays int64
	// NextDay is the next arrival day. When nil it defaults to the current schedule's next day, or to
	// Day + IntervalDays when there is no schedule (interval 7 on day 0 arrives on 7, 14, 21, …).
	NextDay *int64
}

// ScheduleResult is the State to save after a schedule change (SHIPMENT_SCHEDULE_CHANGED, PUBLIC).
type ScheduleResult struct {
	// Before is a copy of the input schedule, nil when there was none; After is nil after a clear.
	Before *Schedule
	After  *Schedule
	State  State
}

// SetSchedule sets the interval and next arrival day. The next arrival must be after the current day
// and fit in an integer, so the countdown is always at least 1 and the old stall can't recur.
func SetSchedule(state State, input ScheduleInput) (ScheduleResult, error) {
	if err := checkStoredState(state); err != nil {
		return ScheduleResult{}, err
	}
	if input.IntervalDays < 1 {
		return ScheduleResult{}, invalid("intervalDays", MessageIntervalTooSmall)
	}
	if input.IntervalDays > MaxStored {
		return ScheduleResult{}, invalid("intervalDays", MessageTooManyDays)
	}
	var nextDay int64
	switch {
	case input.NextDay != nil:
		nextDay = *input.NextDay
	case state.Schedule != nil:
		nextDay = state.Schedule.NextDay
	default:
		nextDay = state.Day + input.IntervalDays
	}
	if nextDay <= state.Day {
		return ScheduleResult{}, invalid("nextShipmentDay", nextArrivalMessage(state.Day))
	}
	if nextDay > MaxStored {
		return ScheduleResult{}, invalid("nextShipmentDay", MessageTooManyDays)
	}
	after := state.Clone()
	after.Schedule = &Schedule{IntervalDays: input.IntervalDays, NextDay: nextDay}
	return ScheduleResult{Before: cloneSchedule(state.Schedule), After: cloneSchedule(after.Schedule), State: after}, nil
}

// ClearSchedule removes the schedule, which hides the countdown; later advances log no arrivals.
func ClearSchedule(state State) ScheduleResult {
	after := state.Clone()
	after.Schedule = nil
	return ScheduleResult{Before: cloneSchedule(state.Schedule), State: after}
}

// DayCorrection is the State to save after Correct day (DAY_CORRECTED, DM visibility).
type DayCorrection struct {
	FromDay int64
	ToDay   int64
	// Before is a copy of the input schedule, After the rolled-forward one (both nil with no schedule).
	Before *Schedule
	After  *Schedule
	// SkippedShipments counts the arrival dates the day jumped past without delivering
	// ("Day 10 → 30 (3 shipment dates skipped)").
	SkippedShipments int64
	State            State
}

// CorrectDay sets the day directly, with no upkeep and no arrivals (spec §5.3). Moving forward past
// the next arrival rolls it to the first D + k·I after the new day; moving backward leaves it alone.
func CorrectDay(state State, newDay int64) (DayCorrection, error) {
	if err := checkStoredState(state); err != nil {
		return DayCorrection{}, err
	}
	if newDay < 0 {
		return DayCorrection{}, invalid("day", MessageNegativeDay)
	}
	if newDay > MaxStored {
		return DayCorrection{}, invalid("day", MessageTooManyDays)
	}
	after := state.Clone()
	after.Day = newDay
	var skipped int64
	if schedule := after.Schedule; schedule != nil && schedule.NextDay <= newDay {
		skipped = (newDay-schedule.NextDay)/schedule.IntervalDays + 1
		schedule.NextDay += skipped * schedule.IntervalDays
		if schedule.NextDay > MaxStored {
			return DayCorrection{}, invalid("day", MessageTooManyDays)
		}
	}
	return DayCorrection{
		FromDay:          state.Day,
		ToDay:            newDay,
		Before:           cloneSchedule(state.Schedule),
		After:            cloneSchedule(after.Schedule),
		SkippedShipments: skipped,
		State:            after,
	}, nil
}

func nextArrivalMessage(day int64) string {
	return "The next shipment must arrive after Day " + strconv.FormatInt(day, 10)
}

func cloneSchedule(schedule *Schedule) *Schedule {
	if schedule == nil {
		return nil
	}
	clone := *schedule
	return &clone
}
