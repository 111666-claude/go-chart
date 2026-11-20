package chart

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/wcharczuk/go-chart/v2/drawing"
	"github.com/wcharczuk/go-chart/v2/testutil"
)

// pathOp is a recorded MoveTo/LineTo operation.
type pathOp struct {
	op   string
	x, y int
}

// mockPathRenderer is a no-op renderer that records path operations.
type mockPathRenderer struct {
	ops []pathOp
}

func (m *mockPathRenderer) ResetStyle()                            {}
func (m *mockPathRenderer) GetDPI() float64                        { return DefaultDPI }
func (m *mockPathRenderer) SetDPI(dpi float64)                     {}
func (m *mockPathRenderer) SetClassName(string)                    {}
func (m *mockPathRenderer) SetStrokeColor(drawing.Color)           {}
func (m *mockPathRenderer) SetFillColor(drawing.Color)             {}
func (m *mockPathRenderer) SetStrokeWidth(width float64)           {}
func (m *mockPathRenderer) SetStrokeDashArray(dashArray []float64) {}
func (m *mockPathRenderer) MoveTo(x, y int) {
	m.ops = append(m.ops, pathOp{op: "M", x: x, y: y})
}
func (m *mockPathRenderer) LineTo(x, y int) {
	m.ops = append(m.ops, pathOp{op: "L", x: x, y: y})
}
func (m *mockPathRenderer) QuadCurveTo(cx, cy, x, y int)                       {}
func (m *mockPathRenderer) ArcTo(cx, cy int, rx, ry, startAngle, delta float64) {}
func (m *mockPathRenderer) Close()                                             {}
func (m *mockPathRenderer) Stroke()                                            {}
func (m *mockPathRenderer) Fill()                                              {}
func (m *mockPathRenderer) FillStroke()                                        {}
func (m *mockPathRenderer) Circle(radius float64, x, y int)                    {}
func (m *mockPathRenderer) SetFont(*truetype.Font)                             {}
func (m *mockPathRenderer) SetFontColor(drawing.Color)                         {}
func (m *mockPathRenderer) SetFontSize(size float64)                           {}
func (m *mockPathRenderer) Text(body string, x, y int)                         {}
func (m *mockPathRenderer) MeasureText(body string) Box                        { return Box{} }
func (m *mockPathRenderer) SetTextRotation(radians float64)                    {}
func (m *mockPathRenderer) ClearTextRotation()                                 {}
func (m *mockPathRenderer) Save(w io.Writer) error                             { return nil }

func TestBrokenRangeNormalization(t *testing.T) {
	// replaced new assertions helper
	r := BrokenRange{
		Min: 0, Max: 100, Domain: 100,
		Breaks: []AxisBreak{
			{Start: 30, End: 10, Width: 5},  // inverted; swapped
			{Start: 15, End: 25, Width: 7},  // overlaps the above; merged
			{Start: 50, End: 50},            // empty; dropped
			{Start: -50, End: -10},          // fully outside; dropped
			{Start: 90, End: 200, Width: 9}, // clamped to max
		},
	}

	breaks := r.normalizedBreaks()
	testutil.AssertEqual(t, 2, len(breaks))

	// merged [10,30] with the max width of the merged breaks.
	testutil.AssertEqual(t, 10.0, breaks[0].Start)
	testutil.AssertEqual(t, 30.0, breaks[0].End)
	testutil.AssertEqual(t, 7, breaks[0].Width)

	// clamped [90,100].
	testutil.AssertEqual(t, 90.0, breaks[1].Start)
	testutil.AssertEqual(t, 100.0, breaks[1].End)
	testutil.AssertEqual(t, 9, breaks[1].Width)
}

func TestBrokenRangeTranslate(t *testing.T) {
	// replaced new assertions helper
	// range [0,36] on a 240px domain with breaks [10,20] and [25,30],
	// each 20px wide. visible delta = 36-15 = 21, visible domain = 200.
	r := BrokenRange{
		Min: 0, Max: 36, Domain: 240,
		Breaks: []AxisBreak{
			{Start: 10, End: 20, Width: 20},
			{Start: 25, End: 30, Width: 20},
		},
	}

	testutil.AssertEqual(t, 0, r.Translate(0))
	testutil.AssertEqual(t, 48, r.Translate(5))   // 5/21*200 ~= 47.6
	testutil.AssertEqual(t, 96, r.Translate(10))  // left edge of gap one
	testutil.AssertEqual(t, 116, r.Translate(20)) // right edge of gap one (96+20)
	testutil.AssertEqual(t, 163, r.Translate(25)) // left edge of gap two
	testutil.AssertEqual(t, 183, r.Translate(30)) // right edge of gap two (163+20)
	testutil.AssertEqual(t, 240, r.Translate(36))

	// the gaps are exactly their configured pixel widths.
	testutil.AssertEqual(t, 20, r.Translate(20)-r.Translate(10))
	testutil.AssertEqual(t, 20, r.Translate(30)-r.Translate(25))

	// values inside hidden intervals project onto the nearest edge.
	testutil.AssertEqual(t, 96, r.Translate(15))  // equidistant; ties to start edge
	testutil.AssertEqual(t, 116, r.Translate(17)) // closer to the end edge
	testutil.AssertEqual(t, 163, r.Translate(26))
	testutil.AssertEqual(t, 183, r.Translate(29))

	// hidden checks.
	testutil.AssertTrue(t, r.IsHidden(15))
	testutil.AssertFalse(t, r.IsHidden(10))
	testutil.AssertFalse(t, r.IsHidden(20))
	testutil.AssertFalse(t, r.IsHidden(5))

	// break-between checks.
	testutil.AssertTrue(t, r.HasBreakBetween(9, 21))
	testutil.AssertTrue(t, r.HasBreakBetween(10, 20))
	testutil.AssertTrue(t, r.HasBreakBetween(15, 22))
	testutil.AssertFalse(t, r.HasBreakBetween(0, 10))
	testutil.AssertFalse(t, r.HasBreakBetween(20, 25))
	testutil.AssertFalse(t, r.HasBreakBetween(1, 2))
}

func TestBrokenRangeTranslateNoBreaksMatchesContinuous(t *testing.T) {
	// replaced new assertions helper
	br := BrokenRange{Min: 1, Max: 8, Domain: 1000}
	cr := ContinuousRange{Min: 1, Max: 8, Domain: 1000}
	for _, v := range []float64{1.0, 2.0, 2.5, 2.7, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0} {
		testutil.AssertEqual(t, cr.Translate(v), br.Translate(v))
	}

	// a fully normalized-away break set behaves the same.
	br.Breaks = []AxisBreak{{Start: 4, End: 4}, {Start: -10, End: -5}}
	for _, v := range []float64{1.0, 2.0, 2.5, 2.7, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0} {
		testutil.AssertEqual(t, cr.Translate(v), br.Translate(v))
	}
}

func TestBrokenRangeTranslateAllHidden(t *testing.T) {
	// replaced new assertions helper
	r := BrokenRange{
		Min: 0, Max: 36, Domain: 240,
		Breaks: []AxisBreak{{Start: -10, End: 100, Width: 20}},
	}
	testutil.AssertEqual(t, 0, r.Translate(0))
	testutil.AssertEqual(t, 0, r.Translate(10))
	testutil.AssertEqual(t, 0, r.Translate(36))
}

func TestBrokenRangeTranslateNarrowDomain(t *testing.T) {
	// replaced new assertions helper
	// the gaps (2x20px) exceed the 30px domain; results stay in [0,30].
	r := BrokenRange{
		Min: 0, Max: 36, Domain: 30,
		Breaks: []AxisBreak{
			{Start: 10, End: 20, Width: 20},
			{Start: 25, End: 30, Width: 20},
		},
	}
	testutil.AssertEqual(t, 0, r.Translate(0))
	testutil.AssertEqual(t, 20, r.Translate(20))
	testutil.AssertEqual(t, 30, r.Translate(36))
	for _, v := range []float64{0, 5, 10, 15, 20, 25, 30, 36} {
		translated := r.Translate(v)
		testutil.AssertTrue(t, translated >= 0 && translated <= 30)
	}
}

func TestBrokenRangeTranslateDescending(t *testing.T) {
	// replaced new assertions helper
	r := BrokenRange{
		Min: 0, Max: 36, Domain: 240, Descending: true,
		Breaks: []AxisBreak{
			{Start: 10, End: 20, Width: 20},
			{Start: 25, End: 30, Width: 20},
		},
	}
	testutil.AssertEqual(t, 240, r.Translate(0))
	testutil.AssertEqual(t, 240-96, r.Translate(10))
	testutil.AssertEqual(t, 240-116, r.Translate(20))
	testutil.AssertEqual(t, 0, r.Translate(36))
}

func TestLineSeriesWithBreaks(t *testing.T) {
	// replaced new assertions helper
	xr := &BrokenRange{
		Min: 0, Max: 36, Domain: 240,
		Breaks: []AxisBreak{
			{Start: 10, End: 20, Width: 20},
			{Start: 25, End: 30, Width: 20},
		},
	}
	yr := &ContinuousRange{Min: 0, Max: 10, Domain: 100}
	canvasBox := Box{Top: 0, Left: 0, Right: 240, Bottom: 100}

	cs := ContinuousSeries{
		XValues: []float64{0, 5, 10, 15, 20, 25, 30, 36},
		YValues: []float64{5, 5, 5, 5, 5, 5, 5, 5},
	}

	mr := &mockPathRenderer{}
	Draw.LineSeries(mr, canvasBox, xr, yr, Style{
		StrokeColor: drawing.ColorBlack,
		StrokeWidth: 1,
	}, cs)

	// y for value 5.0 is 100 - 50 = 50 for every point.
	expected := []pathOp{
		{op: "M", x: 0, y: 50},   // segment one starts
		{op: "L", x: 48, y: 50},  // x=5
		{op: "L", x: 96, y: 50},  // x=10, left edge of gap one
		{op: "M", x: 96, y: 50},  // x=15, hidden, projected to start edge
		{op: "M", x: 116, y: 50}, // x=20, right edge of gap one
		{op: "L", x: 163, y: 50}, // x=25, left edge of gap two
		{op: "M", x: 183, y: 50}, // x=30, right edge of gap two
		{op: "L", x: 240, y: 50}, // x=36
	}
	testutil.AssertEqual(t, len(expected), len(mr.ops))
	for i, op := range expected {
		testutil.AssertEqual(t, op, mr.ops[i])
	}

	// no line crosses a gap: every "L" op starts from the previous point
	// within the same visible region.
	gapOne := [2]int{96, 116}
	gapTwo := [2]int{163, 183}
	for i := 1; i < len(mr.ops); i++ {
		if mr.ops[i].op != "L" {
			continue
		}
		x0, x1 := mr.ops[i-1].x, mr.ops[i].x
		for _, gap := range [][2]int{gapOne, gapTwo} {
			crosses := (x0 <= gap[0] && x1 >= gap[1]) || (x1 <= gap[0] && x0 >= gap[1])
			testutil.AssertFalse(t, crosses)
		}
	}
}

func TestLineSeriesWithoutBreaksUnchanged(t *testing.T) {
	// replaced new assertions helper
	// a plain continuous range produces a single unbroken polyline.
	xr := &ContinuousRange{Min: 0, Max: 36, Domain: 240}
	yr := &ContinuousRange{Min: 0, Max: 10, Domain: 100}
	canvasBox := Box{Top: 0, Left: 0, Right: 240, Bottom: 100}

	cs := ContinuousSeries{
		XValues: []float64{0, 5, 10, 15, 20, 25, 30, 36},
		YValues: []float64{5, 5, 5, 5, 5, 5, 5, 5},
	}

	mr := &mockPathRenderer{}
	Draw.LineSeries(mr, canvasBox, xr, yr, Style{
		StrokeColor: drawing.ColorBlack,
		StrokeWidth: 1,
	}, cs)

	testutil.AssertEqual(t, 8, len(mr.ops))
	testutil.AssertEqual(t, "M", mr.ops[0].op)
	for i := 1; i < len(mr.ops); i++ {
		testutil.AssertEqual(t, "L", mr.ops[i].op)
	}
}

func TestBrokenRangeTicks(t *testing.T) {
	// replaced new assertions helper
	// fixed time axis: 2026-01-01 08:00 + 58h, with two hidden nights.
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	hour := time.Hour

	nightOneStart := TimeToFloat64(start.Add(10 * hour)) // 2026-01-01 18:00
	nightOneEnd := TimeToFloat64(start.Add(24 * hour))   // 2026-01-02 08:00
	nightTwoStart := TimeToFloat64(start.Add(34 * hour)) // 2026-01-02 18:00
	nightTwoEnd := TimeToFloat64(start.Add(48 * hour))   // 2026-01-03 08:00

	xr := &BrokenRange{
		Min:    TimeToFloat64(start),
		Max:    TimeToFloat64(start.Add(58 * hour)), // 2026-01-03 18:00
		Domain: 400,
		Breaks: []AxisBreak{
			{Start: nightOneStart, End: nightOneEnd, Width: 20},
			{Start: nightTwoStart, End: nightTwoEnd, Width: 20},
		},
	}

	mr := &mockPathRenderer{}
	ticks := GenerateContinuousTicks(mr, xr, false, Style{}, TimeValueFormatter)
	testutil.AssertNotZero(t, len(ticks))

	// no generated tick falls inside a hidden interval, and every
	// label is the real (uncompressed) time of its value.
	for _, tick := range ticks {
		testutil.AssertFalse(t, xr.IsHidden(tick.Value))
		testutil.AssertEqual(t, TimeValueFormatter(tick.Value), tick.Label)
	}

	// the first and last ticks are the real range bounds.
	testutil.AssertEqual(t, xr.GetMin(), ticks[0].Value)
	testutil.AssertEqual(t, xr.GetMax(), ticks[len(ticks)-1].Value)
}

func TestChartWithXAxisBreaks(t *testing.T) {
	// replaced new assertions helper
	// fixed series: hourly points 2026-01-01 08:00 -> 2026-01-03 18:00,
	// crossing two hidden nights (18:00 -> 08:00).
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	hour := time.Hour

	var xvalues []time.Time
	var yvalues []float64
	for i := 0; i <= 58; i++ {
		xvalues = append(xvalues, start.Add(time.Duration(i)*hour))
		yvalues = append(yvalues, float64(i%7))
	}

	breaks := []AxisBreak{
		{Start: TimeToFloat64(start.Add(10 * hour)), End: TimeToFloat64(start.Add(24 * hour)), Width: 20},
		{Start: TimeToFloat64(start.Add(34 * hour)), End: TimeToFloat64(start.Add(48 * hour)), Width: 20},
	}

	graph := Chart{
		Width:  512,
		Height: 512,
		XAxis: XAxis{
			Name:   "Time",
			Breaks: breaks,
		},
		YAxis: YAxis{Name: "Value"},
		Series: []Series{
			TimeSeries{XValues: xvalues, YValues: yvalues},
			TimeSeries{XValues: xvalues, YValues: append([]float64(nil), yvalues...), YAxis: YAxisSecondary},
		},
	}

	// the x-range is wrapped in a BrokenRange carrying the axis breaks.
	xr, _, _ := graph.getRanges()
	br, isBrokenRange := xr.(*BrokenRange)
	testutil.AssertTrue(t, isBrokenRange)
	testutil.AssertEqual(t, 2, len(br.Breaks))

	// coordinate checks against the full domain once set.
	xr.SetDomain(400)
	testutil.AssertEqual(t, 0, xr.Translate(TimeToFloat64(start)))
	testutil.AssertEqual(t, 120, xr.Translate(TimeToFloat64(start.Add(10*hour))))
	testutil.AssertEqual(t, 140, xr.Translate(TimeToFloat64(start.Add(24*hour))))
	testutil.AssertEqual(t, 260, xr.Translate(TimeToFloat64(start.Add(34*hour))))
	testutil.AssertEqual(t, 280, xr.Translate(TimeToFloat64(start.Add(48*hour))))
	testutil.AssertEqual(t, 400, xr.Translate(TimeToFloat64(start.Add(58*hour))))

	// each night occupies exactly 20px.
	testutil.AssertEqual(t, 20, xr.Translate(TimeToFloat64(start.Add(24*hour)))-xr.Translate(TimeToFloat64(start.Add(10*hour))))
	testutil.AssertEqual(t, 20, xr.Translate(TimeToFloat64(start.Add(48*hour)))-xr.Translate(TimeToFloat64(start.Add(34*hour))))

	// svg output; deterministic across renders.
	var svgA, svgB bytes.Buffer
	testutil.AssertNil(t, graph.Render(SVG, &svgA))
	testutil.AssertNil(t, graph.Render(SVG, &svgB))
	testutil.AssertNotZero(t, svgA.Len())
	testutil.AssertEqual(t, svgA.String(), svgB.String())

	// png output.
	var png bytes.Buffer
	testutil.AssertNil(t, graph.Render(PNG, &png))
	testutil.AssertNotZero(t, png.Len())

	// without breaks configured the output is the original chart.
	plain := Chart{
		Width:  512,
		Height: 512,
		XAxis:  XAxis{Name: "Time"},
		YAxis:  YAxis{Name: "Value"},
		Series: []Series{
			TimeSeries{XValues: xvalues, YValues: yvalues},
		},
	}
	plainXr, _, _ := plain.getRanges()
	_, plainIsBroken := plainXr.(*BrokenRange)
	testutil.AssertFalse(t, plainIsBroken)

	var plainSVG bytes.Buffer
	testutil.AssertNil(t, plain.Render(SVG, &plainSVG))
	testutil.AssertNotZero(t, plainSVG.Len())
	testutil.AssertNotEqual(t, svgA.String(), plainSVG.String())
}
