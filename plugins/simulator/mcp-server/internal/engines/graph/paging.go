package graph

import "fmt"

// maxLayerPages bounds a paged layer read. Reads go on until an empty page (a short page can
// sit mid-layer), so a server that ignored offset would otherwise make them loop forever.
var maxLayerPages = 10_000

func errTooManyPages() error {
	return fmt.Errorf("layer read stopped after %d pages; the server may be ignoring offset", maxLayerPages)
}
