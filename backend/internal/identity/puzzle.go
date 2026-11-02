package identity

import (
	"strconv"
	"sync"

	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

// The answer stays server-side; the client receives only rendered images and piece geometry.
type Puzzle struct {
	Image       string `json:"image"`
	Piece       string `json:"piece"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	PieceWidth  int    `json:"piece_width"`
	PieceHeight int    `json:"piece_height"`
	Y           int    `json:"y"`
}

var puzzleGenerator = sync.OnceValues(func() (slide.Captcha, error) {
	backgrounds, err := imagesv2.GetImages()
	if err != nil {
		return nil, err
	}
	shapes, err := tiles.GetTiles()
	if err != nil {
		return nil, err
	}
	graphs := make([]*slide.GraphImage, 0, len(shapes))
	for _, shape := range shapes {
		graphs = append(graphs, &slide.GraphImage{OverlayImage: shape.OverlayImage, MaskImage: shape.MaskImage, ShadowImage: shape.ShadowImage})
	}
	builder := slide.NewBuilder(slide.WithImageSize(option.Size{Width: 300, Height: 160}), slide.WithRangeGraphSize(option.RangeVal{Min: 44, Max: 48}))
	builder.SetResources(slide.WithBackgrounds(backgrounds), slide.WithGraphImages(graphs))
	return builder.Make(), nil
})

func newPuzzle() (*Puzzle, string, error) {
	generator, err := puzzleGenerator()
	if err != nil {
		return nil, "", err
	}
	data, err := generator.Generate()
	if err != nil {
		return nil, "", err
	}
	image, err := data.GetMasterImage().ToBase64()
	if err != nil {
		return nil, "", err
	}
	piece, err := data.GetTileImage().ToBase64()
	if err != nil {
		return nil, "", err
	}
	block := data.GetData()
	return &Puzzle{Image: image, Piece: piece, Width: 300, Height: 160, PieceWidth: block.Width, PieceHeight: block.Height, Y: block.Y}, strconv.Itoa(block.X), nil
}
