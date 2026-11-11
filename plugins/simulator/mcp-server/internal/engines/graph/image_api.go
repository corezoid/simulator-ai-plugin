package graph

import "context"

// UploadImage stores an image in the workspace and returns its storage name,
// which an actor takes as its `picture`. An SVG is rasterised to a PNG of the
// given size first (0 = 256), as the graph UI shows raster pictures only.
func UploadImage(ctx context.Context, accID, filename string, data []byte, size int) (string, error) {
	data, filename, contentType, err := maybeConvertSVG(data, filename, size, size, "")
	if err != nil {
		return "", err
	}
	return uploadFile(ctx, accID, filename, contentType, data)
}

// FetchImage downloads an image from a URL, with the same limits as
// uploadActorPicture. It returns the bytes and their content type.
func FetchImage(ctx context.Context, url string) ([]byte, string, error) {
	return fetchImageFromURL(ctx, url)
}

// ReadLocalImage reads an image file, refusing anything that is not an image
// and any path on the hosted server.
func ReadLocalImage(path string) ([]byte, error) { return readLocalImage(path) }
