package core

import (
	"context"
	"fmt"

	"github.com/BBruington/colony-planner/internal/store"
)

// Terrain is the hex map's terrain enum (spec §5.10), re-exported so only core reaches into store.
type Terrain = store.Terrain

const (
	TerrainForest   = store.TerrainFOREST
	TerrainPlains   = store.TerrainPLAINS
	TerrainWater    = store.TerrainWATER
	TerrainMountain = store.TerrainMOUNTAIN
	TerrainDesert   = store.TerrainDESERT
	TerrainSwamp    = store.TerrainSWAMP
	TerrainTundra   = store.TerrainTUNDRA
	TerrainVolcanic = store.TerrainVOLCANIC
)

// AllTerrainValues lists every terrain, for the API's enum schema.
func AllTerrainValues() []Terrain { return store.AllTerrainValues() }

// Hex is one charted hex of the players' map: pointy-top axial coordinates, a terrain and an optional
// label. Nothing on the map is hidden, and nothing records who charted it (F6).
type Hex struct {
	Q       int64   `json:"q"`
	R       int64   `json:"r"`
	Terrain Terrain `json:"terrain"`
	Label   string  `json:"label" doc:"Blank when the hex has none"`
}

type HexList struct {
	Hexes []Hex `json:"hexes"`
}

// HexCoordinate names one hex.
type HexCoordinate struct {
	Q int64 `json:"q"`
	R int64 `json:"r"`
}

// PaintedHex is one cell of a stroke.
type PaintedHex struct {
	Q       int64   `json:"q"`
	R       int64   `json:"r"`
	Terrain Terrain `json:"terrain"`
}

// PaintHexesInput is one batch of a stroke: at most HexCellsPerCall cells.
type PaintHexesInput struct {
	Cells []PaintedHex `json:"cells" doc:"1 to 200 cells; a longer stroke is sent in parts"`
}

// EraseHexesInput uncharts cells, terrain and label together: at most HexCellsPerCall cells.
type EraseHexesInput struct {
	Cells []HexCoordinate `json:"cells" doc:"1 to 200 cells"`
}

// HexLabelInput sets a hex's label; blank clears it.
type HexLabelInput struct {
	Label string `json:"label" doc:"At most 40 characters; blank clears the label"`
}

const hexLabelMaxLength = 40

// ListHexes is every charted hex of the campaign.
func (service *Service) ListHexes(requestContext context.Context, viewer Viewer) (HexList, error) {
	if err := requireActive(viewer); err != nil {
		return HexList{}, err
	}
	rows, err := service.queries.ListHexes(requestContext, viewer.campaignID)
	if err != nil {
		return HexList{}, err
	}
	hexes := make([]Hex, 0, len(rows))
	for _, row := range rows {
		hex := Hex{Q: int64(row.Q), R: int64(row.R), Terrain: row.Terrain}
		if row.Label != nil {
			hex.Label = *row.Label
		}
		hexes = append(hexes, hex)
	}
	return HexList{Hexes: hexes}, nil
}

// PaintHexes charts cells or changes their terrain, keeping every label (spec §5.10). Any active member
// may paint; the last write wins and nothing is recorded but the map itself. When a call names a cell
// twice, its last terrain wins.
func (service *Service) PaintHexes(requestContext context.Context, viewer Viewer, input PaintHexesInput) error {
	if err := requireActive(viewer); err != nil {
		return err
	}
	if err := checkCellCount(len(input.Cells)); err != nil {
		return err
	}
	terrainByCell := map[HexCoordinate]Terrain{}
	order := make([]HexCoordinate, 0, len(input.Cells))
	for index, cell := range input.Cells {
		coordinate := HexCoordinate{Q: cell.Q, R: cell.R}
		if err := checkCoordinate(fmt.Sprintf("cells[%d]", index), coordinate); err != nil {
			return err
		}
		if !cell.Terrain.Valid() {
			message := "Pick a terrain."
			return Invalid(message, FieldError{Field: fmt.Sprintf("cells[%d].terrain", index), Message: message})
		}
		if _, seen := terrainByCell[coordinate]; !seen {
			order = append(order, coordinate)
		}
		terrainByCell[coordinate] = cell.Terrain
	}
	parameters := store.PaintHexesParams{CampaignID: viewer.campaignID}
	for _, coordinate := range order {
		parameters.Qs = append(parameters.Qs, int32(coordinate.Q))
		parameters.Rs = append(parameters.Rs, int32(coordinate.R))
		parameters.Terrains = append(parameters.Terrains, string(terrainByCell[coordinate]))
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent strokes can't each pass the count and go over the limit.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		charted, err := queries.CountHexes(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		uncharted, err := queries.CountUnchartedHexes(requestContext, store.CountUnchartedHexesParams{
			CampaignID: viewer.campaignID,
			Qs:         parameters.Qs,
			Rs:         parameters.Rs,
		})
		if err != nil {
			return err
		}
		if charted+uncharted > HexLimit {
			return Conflict("The map has " + formatNumber(charted) + " charted hexes and holds at most " + formatNumber(HexLimit) +
				". Unchart some before charting more.")
		}
		return queries.PaintHexes(requestContext, parameters)
	})
	if err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, TopicHexes)
	return nil
}

// SetHexLabel sets or clears a charted hex's label. An uncharted hex has nothing to label.
func (service *Service) SetHexLabel(requestContext context.Context, viewer Viewer, coordinate HexCoordinate, input HexLabelInput) error {
	if err := requireActive(viewer); err != nil {
		return err
	}
	if err := checkCoordinate("", coordinate); err != nil {
		return err
	}
	label, fieldError := cleanText("label", input.Label, hexLabelMaxLength)
	if fieldError != nil {
		return Invalid("The label needs fixing.", *fieldError)
	}
	parameters := store.SetHexLabelParams{CampaignID: viewer.campaignID, Q: int32(coordinate.Q), R: int32(coordinate.R)}
	if label != "" {
		parameters.Label = &label
	}
	updated, err := service.queries.SetHexLabel(requestContext, parameters)
	if err != nil {
		return err
	}
	if updated == 0 {
		return NotFound("That hex isn't charted. Paint it before labelling it.")
	}
	service.broker.Publish(viewer.campaignID, TopicHexes)
	return nil
}

// EraseHexes uncharts cells: terrain and label go together, and nothing remembers them (F6).
func (service *Service) EraseHexes(requestContext context.Context, viewer Viewer, input EraseHexesInput) error {
	if err := requireActive(viewer); err != nil {
		return err
	}
	if err := checkCellCount(len(input.Cells)); err != nil {
		return err
	}
	parameters := store.EraseHexesParams{CampaignID: viewer.campaignID}
	for index, cell := range input.Cells {
		if err := checkCoordinate(fmt.Sprintf("cells[%d]", index), cell); err != nil {
			return err
		}
		parameters.Qs = append(parameters.Qs, int32(cell.Q))
		parameters.Rs = append(parameters.Rs, int32(cell.R))
	}
	if err := service.queries.EraseHexes(requestContext, parameters); err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, TopicHexes)
	return nil
}

func checkCellCount(count int) error {
	if count == 0 {
		return Invalid("Pick at least one hex.", FieldError{Field: "cells", Message: "Pick at least one hex."})
	}
	if count > HexCellsPerCall {
		message := fmt.Sprintf("Send at most %d hexes at a time.", HexCellsPerCall)
		return Invalid(message, FieldError{Field: "cells", Message: message})
	}
	return nil
}

// checkCoordinate refuses only bad input: the map has no edge, and painting never reaches this bound.
func checkCoordinate(field string, coordinate HexCoordinate) error {
	if coordinate.Q < -HexCoordinateLimit || coordinate.Q > HexCoordinateLimit || coordinate.R < -HexCoordinateLimit || coordinate.R > HexCoordinateLimit {
		message := fmt.Sprintf("Hex coordinates run from -%s to %s.", formatNumber(HexCoordinateLimit), formatNumber(HexCoordinateLimit))
		if field == "" {
			return Invalid(message)
		}
		return Invalid(message, FieldError{Field: field, Message: message})
	}
	return nil
}
