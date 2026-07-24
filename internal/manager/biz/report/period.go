package report

import (
	"fmt"
	"strings"
	"time"

	model "github.com/ongridio/ongrid/internal/manager/model/report"
)

// Period is the [Start, End) time window a report covers. End is
// exclusive at the boundary semantics level but we materialise it as
// the last instant of the window (23:59:59.999...) when persisting so
// "上周日 23:59:59" reads naturally in titles; the data-collection SQL
// uses [Start, End] inclusive on stored timestamps which never hit the
// exact nanosecond boundary in practice.
type Period struct {
	Start time.Time
	End   time.Time
}

// PeriodFor computes the window a report should cover, given the
// schedule kind and the fire time (the moment the evaluator decided to
// generate). All math is done in the schedule's timezone so "weekly"
// means the operator's Monday, not UTC Monday; the returned times carry
// that location.
//
// Boundaries (HLD-014 §已决, period 边界细节):
//   - daily   → trailing 24h ending at fireAt: [fireAt-24h, fireAt)
//   - weekly  → the previous ISO week, Monday 00:00 → Sunday 24:00
//   - monthly → the previous calendar month, 1st 00:00 → next 1st 00:00
//   - custom  → [prevFireAt, fireAt); caller supplies prevFireAt
//
// For custom, prevFireAt must be non-zero (the schedule's LastFireAt);
// a zero prevFireAt falls back to "the 24h before fireAt" so the first
// custom run still produces a sane window instead of an empty one.
func PeriodFor(kind string, fireAt time.Time, loc *time.Location, prevFireAt time.Time) (Period, error) {
	if loc == nil {
		loc = time.UTC
	}
	f := fireAt.In(loc)
	switch kind {
		case model.KindDaily:
			// Trailing 24h ending at the fire moment — ad-hoc "日报" and
			// scheduled daily both cover the most recent day of ops activity
			// relative to when the report is generated/fired.
			end := f
			start := end.Add(-24 * time.Hour)
			return Period{Start: start, End: end}, nil
	case model.KindWeekly:
		// Previous ISO week: this week's Monday minus 7 days.
		thisMon := startOfISOWeek(f)
		start := thisMon.AddDate(0, 0, -7)
		return Period{Start: start, End: thisMon}, nil
	case model.KindMonthly:
		// Previous calendar month.
		firstThis := startOfMonth(f)
		start := firstThis.AddDate(0, -1, 0)
		return Period{Start: start, End: firstThis}, nil
	case model.KindCustom:
		end := f
		start := prevFireAt.In(loc)
		if prevFireAt.IsZero() || !start.Before(end) {
			// First run, or a clock anomaly — default to the trailing 24h
			// so we never emit an empty/backwards window.
			start = end.AddDate(0, 0, -1)
		}
		return Period{Start: start, End: end}, nil
	default:
		return Period{}, fmt.Errorf("report: unknown schedule kind %q", kind)
	}
}

// TitleFor builds the operator-facing report title for a period. Weekly
// gets an ISO week number; daily gets the date; monthly gets the month;
// custom gets a date range. Language-neutral structure — the period
// label is locale-agnostic (numbers + ISO), only the kind word would
// localise, which the SPA does at render time from Kind.
func TitleFor(kind string, p Period, locale string) string {
	en := strings.HasPrefix(strings.ToLower(locale), "en")
	mtr := func(zh, eng string) string {
		if en {
			return eng
		}
		return zh
	}
	switch kind {
		case model.KindDaily:
			if p.Start.Format("2006-01-02") == p.End.Format("2006-01-02") {
				return fmt.Sprintf("%s · %s", mtr("日报", "Daily"), p.End.Format("2006-01-02"))
			}
			return fmt.Sprintf("%s · %s – %s", mtr("日报", "Daily"),
				p.Start.Format("2006-01-02 15:04"), p.End.Format("2006-01-02 15:04"))
	case model.KindWeekly:
		y, w := p.Start.ISOWeek()
		return fmt.Sprintf("%s · %d W%02d (%s – %s)", mtr("周报", "Weekly"), y, w,
			p.Start.Format("01-02"), p.End.AddDate(0, 0, -1).Format("01-02"))
	case model.KindMonthly:
		return fmt.Sprintf("%s · %s", mtr("月报", "Monthly"), p.Start.Format("2006-01"))
	case model.KindCustom:
		return fmt.Sprintf("%s · %s – %s", mtr("报告", "Report"),
			p.Start.Format("2006-01-02 15:04"), p.End.Format("2006-01-02 15:04"))
	default:
		return fmt.Sprintf("%s · %s – %s", mtr("报告", "Report"),
			p.Start.Format("2006-01-02"), p.End.Format("2006-01-02"))
	}
}

// TitleWithScope appends system / environment labels when scope_json narrows coverage.
func TitleWithScope(base, scopeJSON string) string {
	s := ParseScope(scopeJSON)
	var parts []string
	if sys := strings.TrimSpace(s.SystemName); sys != "" {
		parts = append(parts, sys)
	}
	if env := strings.TrimSpace(s.EnvironmentTag); env != "" {
		parts = append(parts, environmentScopeLabel(env))
	}
	if len(parts) == 0 {
		return base
	}
	return base + " · " + strings.Join(parts, " / ")
}

func environmentScopeLabel(tag string) string {
	switch tag {
	case "dev":
		return "开发"
	case "test":
		return "测试"
	case "prod":
		return "生产"
	default:
		return tag
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// startOfISOWeek returns Monday 00:00 of the week containing t.
// Go's Weekday() makes Sunday=0, so we remap to Monday=0 distance.
func startOfISOWeek(t time.Time) time.Time {
	d := startOfDay(t)
	// Days since Monday: (weekday + 6) % 7  (Mon→0, Tue→1, ..., Sun→6)
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}
