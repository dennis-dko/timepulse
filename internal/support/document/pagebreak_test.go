package document_test

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/support/document"
)

// An A4 page and the margin round it, in the points a PDF measures in.
const (
	pageHighPoints   = 297 * 72 / 25.4
	pageMarginPoints = 18 * 72 / 25.4
)

// TestAChartTallerThanAPageFitsOnOneWithItsHeading is the statistics document
// as a telephone sends it. There the hours per day are a list, a row a day, so
// the chart is far taller than it is wide. A month of them came out as a page
// holding the title and the heading, a page holding nothing but its number, and
// the chart running off the foot of a third: the chart went to a new page
// because it did not fit, and the writer broke the page again by itself because
// it did not fit that one either.
func TestAChartTallerThanAPageFitsOnOneWithItsHeading(t *testing.T) {
	t.Parallel()

	out, err := document.Write(document.Document{
		Title:    "Meine Stunden",
		Sections: []document.Section{{Heading: "Stunden pro Tag", Chart: aChart(t, 340, 1200)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	drawnPages := pageContents(t, out)

	if len(drawnPages) != 1 {
		t.Errorf("a tall chart under a title and its heading came to %d pages, want one", len(drawnPages))
	}

	for i, page := range drawnPages {
		if blankPage(page) {
			t.Errorf("page %d holds nothing but its number", i+1)
		}

		for _, box := range imageBoxes(page) {
			if box.bottom < pageMarginPoints-0.5 || box.top > pageHighPoints-pageMarginPoints+0.5 {
				t.Errorf("on page %d a chart runs from %.0fpt to %.0fpt, outside the margins of a %.0fpt page",
					i+1, box.bottom, box.top, pageHighPoints)
			}
		}
	}
}

// TestAHeadingGoesToThePageItsChartGoesTo: a chart that does not fit under what
// is already on a page moves to the next one whole, and its heading has to go
// with it. It used to stay behind at the foot of the page before, heading
// nothing, over a gap the height of the chart.
func TestAHeadingGoesToThePageItsChartGoesTo(t *testing.T) {
	t.Parallel()

	// The first chart fills the page down to some 40mm above its foot, which
	// leaves room for the second heading and none for its chart.
	out, err := document.Write(document.Document{
		Title: "Report",
		Sections: []document.Section{
			{Heading: "One", Chart: aChart(t, 400, 460)},
			{Heading: "Two", Chart: aChart(t, 400, 200)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	drawnPages := pageContents(t, out)

	if len(drawnPages) != 2 {
		t.Fatalf("two charts that do not share a page came to %d pages, want two", len(drawnPages))
	}

	// The second page holds the second chart, and with its heading more than the
	// page's number.
	if imageCount(drawnPages[1]) != 1 || textRuns(drawnPages[1]) < 2 {
		t.Errorf("the second page holds %d charts and %d lines of text, want the second chart under its heading",
			imageCount(drawnPages[1]), textRuns(drawnPages[1]))
	}
}

// pageContents answers what each page of a PDF is drawn with, in the order the
// pages are listed: the operators of its content stream, inflated.
func pageContents(t *testing.T, pdf []byte) []string {
	t.Helper()

	objects := map[string][]byte{}

	for _, m := range regexp.MustCompile(`(?s)(\d+) 0 obj(.*?)endobj`).FindAllSubmatch(pdf, -1) {
		objects[string(m[1])] = m[2]
	}

	kids := regexp.MustCompile(`/Kids \[([^\]]*)\]`).FindSubmatch(pdf)
	if kids == nil {
		t.Fatal("the PDF lists no pages")
	}

	var drawnPages []string

	for _, ref := range regexp.MustCompile(`(\d+) 0 R`).FindAllSubmatch(kids[1], -1) {
		contents := regexp.MustCompile(`/Contents (\d+) 0 R`).FindSubmatch(objects[string(ref[1])])
		if contents == nil {
			t.Fatalf("page object %s has no contents", ref[1])
		}

		body := objects[string(contents[1])]
		start := bytes.Index(body, []byte("stream\n"))
		end := bytes.LastIndex(body, []byte("\nendstream"))

		if start < 0 || end < start {
			t.Fatalf("page object %s has no stream", ref[1])
		}

		stream := body[start+len("stream\n") : end]

		inflated, err := zlib.NewReader(bytes.NewReader(stream))
		if err != nil {
			drawnPages = append(drawnPages, string(stream))

			continue
		}

		read, err := io.ReadAll(inflated)
		if err != nil {
			t.Fatalf("inflating page object %s: %v", ref[1], err)
		}

		drawnPages = append(drawnPages, string(read))
	}

	return drawnPages
}

// textRuns is how many pieces of text a page writes.
func textRuns(page string) int { return strings.Count(page, "Tj") + strings.Count(page, "TJ") }

// imageCount is how many pictures a page places.
func imageCount(page string) int { return strings.Count(page, " Do") }

// blankPage is a page with no picture on it and no text but its number, which
// is all the footer writes for a document without footer text or a date.
func blankPage(page string) bool { return imageCount(page) == 0 && textRuns(page) <= 1 }

type imageBox struct{ bottom, top float64 }

// imageBoxes are where a page's pictures stand, measured up from the foot of the
// page, as the writer places them: "q w 0 0 h x y cm /name Do Q".
func imageBoxes(page string) []imageBox {
	var boxes []imageBox

	for _, m := range regexp.MustCompile(`q (-?[\d.]+) 0 0 (-?[\d.]+) (-?[\d.]+) (-?[\d.]+) cm /\w+ Do Q`).FindAllStringSubmatch(page, -1) {
		high, _ := strconv.ParseFloat(m[2], 64)
		bottom, _ := strconv.ParseFloat(m[4], 64)
		boxes = append(boxes, imageBox{bottom: bottom, top: bottom + high})
	}

	return boxes
}
