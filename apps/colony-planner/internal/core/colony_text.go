package core

import (
	"strconv"
	"strings"

	"github.com/BBruington/colony-planner/internal/game"
)

// Timeline text is rendered when an entry is written, in player-facing words (spec §5.8). These
// helpers keep every colony entry and preview in one voice: thousands separators, a true minus sign,
// "Day 44".

// formatNumber writes 1000034 as "1,000,034" and -39 as "−39".
func formatNumber(value int64) string {
	digits := strconv.FormatInt(value, 10)
	negative := strings.HasPrefix(digits, "-")
	digits = strings.TrimPrefix(digits, "-")
	var builder strings.Builder
	if negative {
		builder.WriteString("−")
	}
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(digit)
	}
	return builder.String()
}

// formatDelta writes a change with its sign: "+20", "−39".
func formatDelta(delta int64) string {
	if delta > 0 {
		return "+" + formatNumber(delta)
	}
	return formatNumber(delta)
}

func dayText(day int64) string {
	return "Day " + formatNumber(day)
}

func plural(count int64, singular string, pluralForm string) string {
	if count == 1 {
		return singular
	}
	return pluralForm
}

func colonistsText(count int64) string {
	return formatNumber(count) + " " + plural(count, "colonist", "colonists") + " (Unassigned)"
}

// resourceLabels names each resource for text, by key; a key with no row keeps the key itself.
type resourceLabels map[string]string

func (labels resourceLabels) of(key string) string {
	if label, found := labels[key]; found {
		return label
	}
	return key
}

// appliedEffectsText is the body line for applied effects, in board order: "Food +20 · Morale +2 ·
// 3 colonists (Unassigned)".
func appliedEffectsText(applied Effects, order []string, labels resourceLabels) string {
	parts := make([]string, 0, len(applied.Resources)+1)
	for _, key := range order {
		if delta, found := applied.Resources[key]; found && delta != 0 {
			parts = append(parts, labels.of(key)+" "+formatDelta(delta))
		}
	}
	if applied.Colonists > 0 {
		parts = append(parts, colonistsText(applied.Colonists))
	}
	return strings.Join(parts, " · ")
}

// arrivalDaysText lists arrival runs: "Day 14", "Days 7, 14, 21", or "Days 42 – 1,000,034, every 7".
func arrivalDaysText(runs []game.ArrivalRun) string {
	var total int64
	for _, run := range runs {
		total += run.Count
	}
	if total == 0 {
		return ""
	}
	if total == 1 {
		return dayText(runs[0].First)
	}
	if total > game.MaxUnfoldedArrivals {
		parts := make([]string, 0, len(runs))
		for _, run := range runs {
			parts = append(parts, "Days "+formatNumber(run.First)+" – "+formatNumber(run.Last())+", every "+formatNumber(run.Every))
		}
		return strings.Join(parts, "; ")
	}
	days := make([]string, 0, total)
	for _, run := range runs {
		for arrival := range run.Count {
			days = append(days, formatNumber(run.First+arrival*run.Every))
		}
	}
	return "Days " + strings.Join(days, ", ")
}

// arrivalsSentence is the DAYS_ADVANCED body's shipment part: "Shipment arrived Day 42",
// "Shipments arrived Days 7, 14", or the folded "142,857 shipments arrived, every 7 days, Days 42 –
// 1,000,034".
func arrivalsSentence(runs []game.ArrivalRun, count int64) string {
	switch {
	case count == 0:
		return ""
	case count == 1:
		return "Shipment arrived " + arrivalDaysText(runs)
	case count > game.MaxUnfoldedArrivals && len(runs) == 1:
		return foldedArrivalTitle(runs[0])
	default:
		return formatNumber(count) + " shipments arrived, " + arrivalDaysText(runs)
	}
}

func foldedArrivalTitle(run game.ArrivalRun) string {
	return formatNumber(run.Count) + " shipments arrived, every " + formatNumber(run.Every) + " " +
		plural(run.Every, "day", "days") + ", Days " + formatNumber(run.First) + " – " + formatNumber(run.Last())
}

func daysPassTitle(days int64, newDay int64) string {
	return formatNumber(days) + " " + plural(days, "day passes", "days pass") + " · " + dayText(newDay)
}

func scheduleText(schedule *game.Schedule) string {
	if schedule == nil {
		return "no shipments scheduled"
	}
	return "every " + formatNumber(schedule.IntervalDays) + " " + plural(schedule.IntervalDays, "day", "days") +
		", next " + dayText(schedule.NextDay)
}

// joinParts joins the non-empty parts with " · ".
func joinParts(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " · ")
}
