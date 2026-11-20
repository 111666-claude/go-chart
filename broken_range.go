package chart

import (
	"fmt"
	"math"
	"sort"
)

// DefaultAxisBreakWidth is the default pixel width of a rendered axis break gap.
const DefaultAxisBreakWidth = 20

// Interface Assertions.
var (
	_ Range          = (*BrokenRange)(nil)
	_ BreaksProvider = (*BrokenRange)(nil)
)

// BreaksProvider is implemented by ranges that hide one or more
// intervals of the axis, rendering them as fixed width gaps.
type BreaksProvider interface {
	// IsHidden returns if a value falls within a hidden interval.
	IsHidden(value float64) bool
	// HasBreakBetween returns if a hidden interval falls between two values.
	HasBreakBetween(x0, x1 float64) bool
}

// AxisBreak describes a hidden interval on an axis. The interval
// [Start, End] (in axis values, e.g. unix seconds for time axes) is
// collapsed to a gap of Width pixels when rendered.
type AxisBreak struct {
	Start float64
	End   float64

	// Width is the pixel width of the rendered gap.
	// If unset (or negative) DefaultAxisBreakWidth is used.
	Width int
}

// GetWidth returns the pixel width of the break gap.
func (b AxisBreak) GetWidth() int {
	if b.Width <= 0 {
		return DefaultAxisBreakWidth
	}
	return b.Width
}

// BrokenRange is a Range that hides one or more intervals (breaks),
// rendering each as a fixed width gap. Values inside a hidden interval
// are projected onto the nearest break edge when translated.
//
// Break normalization is well defined for degenerate input:
//   - inverted breaks (Start > End) are swapped,
//   - empty breaks (Start == End) are dropped,
//   - breaks are clamped to [Min, Max],
//   - overlapping (or touching) breaks are merged into a single break
//     whose width is the maximum of the merged widths.
//
// If every value is hidden, or the domain is too narrow to fit the
// gaps, Translate collapses values towards zero (clamped to [0, Domain]).
type BrokenRange struct {
	Min        float64
	Max        float64
	Domain     int
	Descending bool

	Breaks []AxisBreak
}

// IsDescending returns if the range is descending.
func (r BrokenRange) IsDescending() bool {
	return r.Descending
}

// IsZero returns if the BrokenRange has been set or not.
func (r BrokenRange) IsZero() bool {
	return (r.Min == 0 || math.IsNaN(r.Min)) &&
		(r.Max == 0 || math.IsNaN(r.Max)) &&
		r.Domain == 0
}

// GetMin gets the min value for the range.
func (r BrokenRange) GetMin() float64 {
	return r.Min
}

// SetMin sets the min value for the range.
func (r *BrokenRange) SetMin(min float64) {
	r.Min = min
}

// GetMax returns the max value for the range.
func (r BrokenRange) GetMax() float64 {
	return r.Max
}

// SetMax sets the max value for the range.
func (r *BrokenRange) SetMax(max float64) {
	r.Max = max
}

// GetDelta returns the difference between the min and max value.
// Note this includes any hidden intervals.
func (r BrokenRange) GetDelta() float64 {
	return r.Max - r.Min
}

// GetDomain returns the range domain.
func (r BrokenRange) GetDomain() int {
	return r.Domain
}

// SetDomain sets the range domain.
func (r *BrokenRange) SetDomain(domain int) {
	r.Domain = domain
}

// String returns a simple string for the BrokenRange.
func (r BrokenRange) String() string {
	if r.GetDelta() == 0 {
		return "BrokenRange [empty]"
	}
	return fmt.Sprintf("BrokenRange [%.2f,%.2f] => %d with (%d) breaks", r.Min, r.Max, r.Domain, len(r.Breaks))
}

// normalizedBreaks returns the breaks sorted, swapped, clamped to
// [Min, Max] and merged, with default widths applied.
func (r BrokenRange) normalizedBreaks() []AxisBreak {
	var normalized []AxisBreak
	for _, b := range r.Breaks {
		start, end := b.Start, b.End
		if start > end {
			start, end = end, start
		}
		if start == end {
			continue
		}
		start = math.Max(start, r.Min)
		end = math.Min(end, r.Max)
		if start >= end {
			continue
		}
		normalized = append(normalized, AxisBreak{Start: start, End: end, Width: b.GetWidth()})
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Start < normalized[j].Start
	})

	var merged []AxisBreak
	for _, b := range normalized {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if b.Start <= last.End {
				last.End = math.Max(last.End, b.End)
				last.Width = MaxInt(last.Width, b.Width)
				continue
			}
		}
		merged = append(merged, b)
	}
	return merged
}

// IsHidden returns if a value falls (strictly) within a hidden interval.
func (r BrokenRange) IsHidden(value float64) bool {
	for _, b := range r.normalizedBreaks() {
		if value > b.Start && value < b.End {
			return true
		}
	}
	return false
}

// HasBreakBetween returns if a hidden interval falls between two values.
// Values exactly on a break edge do not count as crossing the break.
func (r BrokenRange) HasBreakBetween(x0, x1 float64) bool {
	lo, hi := math.Min(x0, x1), math.Max(x0, x1)
	for _, b := range r.normalizedBreaks() {
		if lo < b.End && hi > b.Start {
			return true
		}
	}
	return false
}

// Translate maps a given value into the BrokenRange space.
//
// Values within a hidden interval are projected onto the nearest break
// edge (ties resolve to the start edge). Each hidden interval occupies
// exactly its configured pixel width in the output.
func (r BrokenRange) Translate(value float64) int {
	breaks := r.normalizedBreaks()
	if len(breaks) == 0 {
		return (ContinuousRange{
			Min:        r.Min,
			Max:        r.Max,
			Domain:     r.Domain,
			Descending: r.Descending,
		}).Translate(value)
	}

	// project values inside hidden intervals onto the nearest edge.
	v := value
	for _, b := range breaks {
		if v > b.Start && v < b.End {
			if v-b.Start <= b.End-v {
				v = b.Start
			} else {
				v = b.End
			}
			break
		}
	}

	var hiddenTotal float64
	var totalBreakWidth int
	for _, b := range breaks {
		hiddenTotal += b.End - b.Start
		totalBreakWidth += b.Width
	}

	visibleDelta := r.GetDelta() - hiddenTotal
	visibleDomain := MaxInt(r.Domain-totalBreakWidth, 0)

	if visibleDelta <= 0 {
		// everything is hidden; collapse to the origin.
		if r.IsDescending() {
			return r.Domain
		}
		return 0
	}

	visible := v - r.Min
	widthBefore := 0
	for _, b := range breaks {
		if b.End <= v {
			visible -= b.End - b.Start
			widthBefore += b.Width
		} else {
			break
		}
	}

	ratio := visible / visibleDelta
	translated := widthBefore + int(math.Ceil(ratio*float64(visibleDomain)))
	translated = MinInt(MaxInt(translated, 0), r.Domain)

	if r.IsDescending() {
		return r.Domain - translated
	}
	return translated
}
