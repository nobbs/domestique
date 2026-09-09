package zwift

import (
	"encoding/json"
	"slices"
)

// World is one Zwift virtual world: the corners its rides' coordinates fall
// between, and the map artwork Zwift publishes for it. North-west and
// south-east corners, so North > South and West < East — no world is anywhere
// near the antimeridian.
type World struct {
	Name string
	// ImageFile is the artwork's name under the CDN's map directory, never a
	// whole URL: the relay composes the rest.
	ImageFile string
	ID        int64
	North     float64
	West      float64
	South     float64
	East      float64
	// ImageQuarterTurns is how many quarter turns clockwise the published
	// artwork has to be turned before the corners above describe it. Zwift
	// draws the newer worlds' minimaps a quarter turn from the frame their
	// coordinates are quoted in, so a ride placed on the artwork untouched
	// lands nowhere near its roads.
	ImageQuarterTurns int
}

// worldTable is the published bounds and artwork of every Zwift world, copied
// from the zwift-data package's worlds table rather than read at runtime.
// There is no world 12.
//
// The quarter turns are this service's own: zwift-data quotes its bounds
// against artwork a quarter turn from what Zwift's CDN publishes, which is why
// zwiftmap — the same author — ships pre-turned copies rather than the CDN's.
// Each is measured, not guessed: the turn scoring highest is the one that puts
// a real ride's recorded track on the artwork's own roads, and it wins by the
// whole range (1.00 against 0.02 for the next best).
func worldTable() []World {
	return []World{
		{ID: 1, Name: "Watopia", ImageFile: "MiniMap_Watopia_2.png",
			North: -11.62597, West: 166.87747, South: -11.74087, East: 167.03255},
		{ID: 2, Name: "Richmond", ImageFile: "MiniMap_Richmond.png",
			North: 37.5774, West: -77.48954, South: 37.5014, East: -77.394},
		{ID: 3, Name: "London", ImageFile: "MiniMap_London.png",
			North: 51.5362, West: -0.1776, South: 51.4601, East: -0.0555},
		{ID: 4, Name: "New York", ImageFile: "MiniMap_NewYork_2.png",
			North: 40.817257, West: -74.022655, South: 40.587913, East: -73.922165},
		{ID: 5, Name: "Innsbruck", ImageFile: "MiniMap_Innsbruck.png",
			North: 47.2947, West: 11.3501, South: 47.2055, East: 11.4822},
		{ID: 6, Name: "Bologna", ImageFile: "MiniMap_Bologna.png",
			North: 44.5308037, West: 11.26261748, South: 44.45463821, East: 11.36991729102076},
		{ID: 7, Name: "Yorkshire", ImageFile: "MiniMap_Yorkshire.png",
			North: 54.0254, West: -1.632, South: 53.9491, East: -1.5022},
		{ID: 8, Name: "Crit City", ImageFile: "MiniMap_CritCity.png",
			North: -10.3657, West: 165.7824, South: -10.4038, East: 165.8207},
		{ID: 9, Name: "Makuri Islands", ImageFile: "MiniMap_Japan.png",
			North: -10.73746, West: 165.76591, South: -10.85234, East: 165.88222,
			ImageQuarterTurns: 3},
		{ID: 10, Name: "France", ImageFile: "MiniMap_France.png",
			North: -21.64155, West: 166.1384, South: -21.7564, East: 166.26125,
			ImageQuarterTurns: 3},
		{ID: 11, Name: "Paris", ImageFile: "MiniMap_Champs.png",
			North: 48.9058, West: 2.2561, South: 48.82945, East: 2.3722,
			ImageQuarterTurns: 3},
		{ID: 13, Name: "Scotland", ImageFile: "MiniMap_Scotland.png",
			North: 55.67595, West: -5.2802, South: 55.61845, East: -5.17798,
			ImageQuarterTurns: 3},
	}
}

// WorldByID is the world with that id, and whether there is one.
func WorldByID(id int64) (World, bool) {
	table := worldTable()
	index := slices.IndexFunc(table, func(world World) bool { return world.ID == id })
	if index < 0 {
		return World{}, false
	}

	return table[index], true
}

// WorldOf is the world one stored activity summary was ridden in. The document
// is this package's own, written by Activity.Summary; a summary from anywhere
// else, or one naming a world this table does not know, has none.
func WorldOf(summary []byte) (World, bool) {
	var document struct {
		WorldID int64 `json:"worldId"`
	}
	if err := json.Unmarshal(summary, &document); err != nil {
		return World{}, false
	}

	return WorldByID(document.WorldID)
}
