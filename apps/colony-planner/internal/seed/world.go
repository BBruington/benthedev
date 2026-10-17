package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/BBruington/colony-planner/internal/core"
)

// MainlandArtPath is the Eldari art in the repository, read from its root (where every mise task runs).
// `colony seed` uploads it as Sample Colony's Mainland like any DM's upload; no other campaign gets it
// (C6), and production seeds no map.
const MainlandArtPath = "web/public/eldari.webp"

// sampleColonyPins place the M2 sample places on the Mainland, the hidden Drowned Chapel among them, so
// view as player shows a pin go missing.
var sampleColonyPins = map[string][2]float64{
	"Port Eldari Docks":  {0.62, 0.71},
	"The Longhouse":      {0.55, 0.64},
	"Saltmarsh Fields":   {0.48, 0.72},
	"Wolf Den Hollow":    {0.58, 0.48},
	"The Drowned Chapel": {0.35, 0.80},
}

// sampleColonyHexes is the default charted area: the first two rings around Port Eldari, in spiral order
// (spiralHexes), with a few labels.
var sampleColonyHexes = []core.Terrain{
	core.TerrainPlains,
	core.TerrainWater, core.TerrainWater, core.TerrainPlains, core.TerrainForest, core.TerrainSwamp, core.TerrainPlains,
	core.TerrainWater, core.TerrainWater, core.TerrainWater, core.TerrainPlains, core.TerrainForest, core.TerrainForest,
	core.TerrainForest, core.TerrainMountain, core.TerrainForest, core.TerrainSwamp, core.TerrainSwamp, core.TerrainPlains,
}

var sampleColonyHexLabels = map[int]string{0: "Port Eldari", 5: "Saltmarsh", 13: "Wolf Den", 14: "Grey Tor"}

var secondSampleHexes = []core.Terrain{
	core.TerrainMountain,
	core.TerrainMountain, core.TerrainForest, core.TerrainTundra, core.TerrainMountain, core.TerrainForest, core.TerrainTundra,
}

var secondSampleHexLabels = map[int]string{0: "Ridgehold", 4: "The Old Mine"}

// sampleHexLabelEvery labels one hex in this many of a --hexes map, so the performance check draws labels.
const sampleHexLabelEvery = 100

// seedMainland uploads the Eldari art as Sample Colony's Mainland and pins the sample places on it.
func seedMainland(requestContext context.Context, service *core.Service, dm core.Viewer, art []byte) error {
	if len(art) == 0 {
		return errors.New("the Mainland art is missing; run `colony seed` from the repository root, where " + MainlandArtPath + " is")
	}
	uploaded, err := service.UploadImage(requestContext, dm, art)
	if err != nil {
		return fmt.Errorf("upload the Mainland art: %w", err)
	}
	mainland, err := service.CreateMap(requestContext, dm, core.NewMapInput{
		MapInput:    core.MapInput{Name: "Eldari"},
		MapArtInput: core.MapArtInput{ImageID: &uploaded.ID},
		Revealed:    true,
		Mainland:    true,
	})
	if err != nil {
		return fmt.Errorf("the Mainland: %w", err)
	}
	places, err := service.ListPlaces(requestContext, dm, "")
	if err != nil {
		return err
	}
	for _, place := range places.Places {
		spot, pinned := sampleColonyPins[place.Name]
		if !pinned {
			continue
		}
		if _, err := service.SetPlacePin(requestContext, dm, place.ID, core.PinInput{MapID: mainland.ID, X: spot[0], Y: spot[1]}); err != nil {
			return fmt.Errorf("pin %s: %w", place.Name, err)
		}
	}
	return nil
}

// seedHexes paints the given terrains in spiral order from the origin and sets the labels, through core
// as the players would, a stroke at a time.
func seedHexes(requestContext context.Context, service *core.Service, viewer core.Viewer, terrains []core.Terrain, labels map[int]string) error {
	cells := spiralHexes(len(terrains))
	for start := 0; start < len(cells); start += core.HexCellsPerCall {
		end := min(start+core.HexCellsPerCall, len(cells))
		stroke := make([]core.PaintedHex, 0, end-start)
		for index := start; index < end; index++ {
			stroke = append(stroke, core.PaintedHex{Q: cells[index].Q, R: cells[index].R, Terrain: terrains[index]})
		}
		if err := service.PaintHexes(requestContext, viewer, core.PaintHexesInput{Cells: stroke}); err != nil {
			return fmt.Errorf("paint hexes: %w", err)
		}
	}
	for index, label := range labels {
		if index >= len(cells) {
			continue
		}
		if err := service.SetHexLabel(requestContext, viewer, cells[index], core.HexLabelInput{Label: label}); err != nil {
			return fmt.Errorf("label %s: %w", label, err)
		}
	}
	return nil
}

// manySampleHexes is the --hexes map: the default area first, then rings of every terrain outward.
func manySampleHexes(count int) ([]core.Terrain, map[int]string) {
	allTerrains := core.AllTerrainValues()
	terrains := make([]core.Terrain, count)
	labels := map[int]string{}
	for index := range count {
		switch {
		case index < len(sampleColonyHexes):
			terrains[index] = sampleColonyHexes[index]
			if label, labelled := sampleColonyHexLabels[index]; labelled {
				labels[index] = label
			}
		default:
			terrains[index] = allTerrains[(index/7)%len(allTerrains)]
			if index%sampleHexLabelEvery == 0 {
				labels[index] = fmt.Sprintf("Survey mark %d", index/sampleHexLabelEvery)
			}
		}
	}
	return terrains, labels
}

// spiralHexes lists count axial cells ring by ring around the origin, the order a map grows outward.
func spiralHexes(count int) []core.HexCoordinate {
	directions := []core.HexCoordinate{{Q: 1, R: 0}, {Q: 1, R: -1}, {Q: 0, R: -1}, {Q: -1, R: 0}, {Q: -1, R: 1}, {Q: 0, R: 1}}
	cells := []core.HexCoordinate{{Q: 0, R: 0}}
	for radius := int64(1); len(cells) < count; radius++ {
		cell := core.HexCoordinate{Q: directions[4].Q * radius, R: directions[4].R * radius}
		for _, direction := range directions {
			for range radius {
				cells = append(cells, cell)
				cell = core.HexCoordinate{Q: cell.Q + direction.Q, R: cell.R + direction.R}
			}
		}
	}
	return cells[:count]
}
