package ocr

// The model reads a fixed 1664x2048 page, so events are stacked in bands down
// it. 230 px bands keep neighbouring events from merging into one block.
const (
	PageW   = 1664
	PageH   = 2048
	BandH   = 230
	PerPage = 8
)

// PageCount is the number of pages needed for n events.
func PageCount(n int) int { return (n + PerPage - 1) / PerPage }

// Slot gives the page and band that event i is drawn in.
func Slot(i int) (page, band int) { return i / PerPage, i % PerPage }
