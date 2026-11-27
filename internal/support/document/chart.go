package document

import (
	"bytes"
	"fmt"
	"image/png"
	"strconv"

	"codeberg.org/go-pdf/fpdf"
)

// MaxChartPixels bounds what a chart may decode to, in pixels.
//
// The endpoint bounds the picture at a megabyte, and that says nothing about what
// it costs to read: a PNG compresses by orders of magnitude, and the browser
// sends exactly the kind that costs most. A canvas produces colour type 6, so the
// alpha channel must be split from the colour, and splitting it means inflating
// the whole image.
//
// Measured through Write with a flat RGBA chart rather than estimated: 8000
// square arrives as 260 KB and allocates 782 MB; 12000 square arrives as 566 KB
// and allocates 2.6 GB. Both are inside the byte limit, and extrapolated to it,
// about 16000 square - roughly 5 GB, on machines whose memory is measured in
// single-figure gigabytes.
//
// Sixteen megapixels is far above any chart that exists. app.js draws one at the
// chart card's own box times CHART_SCALE, which is 2, so even a chart running the
// full width of a 5120-pixel screen is around 8 megapixels. It is also far above
// what the page can show: this is placed at the text width, about 170mm, which is
// some 2000 pixels at 300 dpi.
//
// What it leaves is around 290 MB in the worst case, at the same 18 bytes per
// pixel the measurements above work out to. That is not nothing, and it is the
// honest trade: bounding tighter would start refusing a real report from a wide
// screen, and the byte limit that was supposed to cover this cannot, because
// bytes do not predict pixels.
const MaxChartPixels = 16 << 20

// picture is a chart registered with the document: its name there, how high it
// stands at the width of the text, and how high it is drawn.
type picture struct {
	name    string
	natural float64
	high    float64
}

// readChart checks the picture the browser drew and registers it with the
// document, under a name its section's number makes its own.
func readChart(pdf *fpdf.Fpdf, number int, chart []byte) (picture, error) {
	config, err := png.DecodeConfig(bytes.NewReader(chart))
	if err != nil {
		return picture{}, fmt.Errorf("the chart is not a readable PNG: %w", err)
	}

	if config.Width <= 0 || config.Height <= 0 {
		return picture{}, fmt.Errorf("the chart has no size: %dx%d", config.Width, config.Height)
	}

	// The other end of the same question, which was missing. DecodeConfig has
	// already read the header and nothing has been inflated yet, so this refuses
	// before the cost is paid rather than after.
	if config.Width*config.Height > MaxChartPixels {
		return picture{}, fmt.Errorf("the chart is too large at %dx%d pixels; at most %d "+
			"megapixels are accepted", config.Width, config.Height, MaxChartPixels>>20)
	}

	name := "chart-" + strconv.Itoa(number)

	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"},
		bytes.NewReader(chart))

	if pdf.Err() {
		return picture{}, fmt.Errorf("the chart could not be placed: %s", pdf.Error())
	}

	return picture{name: name, natural: contentWide * float64(config.Height) / float64(config.Width)}, nil
}

// chartHeight says how high a chart of natural height is drawn, given the room
// left on the page and the room a fresh page has, and whether it starts a page.
//
// Its own proportions at the width of the text where they fit, so a chart of
// thirty projects is not squashed into the space three needed, and one that
// does not fit under what is already on the page moves to the next one whole:
// half a chart at the foot of a page says less than a gap does. A chart taller
// than any page is drawn smaller rather than run off the foot of one - in the
// room this page has left when that is at least half a page, and on a page of
// its own otherwise. It used to move to the next page and then be moved again by
// fpdf, which found it did not fit there either, leaving a page with nothing on
// it and a chart running past the foot of the next.
func chartHeight(natural, room, full float64) (float64, bool) {
	switch {
	case natural <= room:
		return natural, false
	case natural <= full:
		return natural, true
	case room >= full/2:
		return room, false
	default:
		return full, true
	}
}

// introHeight is how high a section's heading and caption are written, measured
// in the faces they are written in, so the page can be chosen before either is.
func introHeight(pdf *fpdf.Fpdf, heading, caption string) float64 {
	high := 0.0

	if heading != "" {
		pdf.SetFont("go", "B", headingSize)
		high += float64(len(pdf.SplitText(heading, contentWide)))*lineHeight + 1
	}

	if caption != "" {
		pdf.SetFont("go", "", captionSize)
		high += float64(len(pdf.SplitText(caption, contentWide)))*(lineHeight-1) + 1
	}

	return high
}

// placeChart draws a chart where the writer stands, at the height chartHeight
// chose and the width that keeps its proportions. Not flowing: the page has been
// chosen already, and fpdf's own break was the second of the two that left a
// page empty.
func placeChart(pdf *fpdf.Fpdf, chart picture) {
	top := pdf.GetY()

	pdf.ImageOptions(chart.name, pageMargin, top, contentWide*chart.high/chart.natural, chart.high, false,
		fpdf.ImageOptions{ImageType: "PNG"}, 0, "")

	pdf.SetY(top + chart.high)
	pdf.Ln(3)
}
