package core_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// mapTest is a campaign with a player, the DM viewing as player, a pending member and a second
// campaign, for the M4 world-map rules.
type mapTest struct {
	*fixture
	dm         core.Viewer
	player     core.Viewer
	dmAsPlayer core.Viewer
	pending    core.Viewer
	otherDm    core.Viewer
}

func newMapTest(t *testing.T) *mapTest {
	t.Helper()
	test := newFixture(t)
	dmCaller := test.caller(discordID(1), "dm")
	campaignID, dm := test.campaign(dmCaller, "Sample", "sample")
	viewingAsPlayer := dmCaller
	viewingAsPlayer.ViewAsPlayer = true
	_, player := test.player(dm, discordID(2), "ana", true)
	_, pending := test.player(dm, discordID(3), "emil", false)
	_, otherDm := test.campaign(test.caller(discordID(4), "hollis"), "Second", "second")
	return &mapTest{fixture: test, dm: dm, player: player, dmAsPlayer: test.enter(viewingAsPlayer, campaignID), pending: pending, otherDm: otherDm}
}

// pngOf encodes a width×height PNG, padded after its end so it reaches size bytes (the header is all
// the server reads) and so each seed gives different bytes.
func pngOf(t *testing.T, width int, height int, seed int, size int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.RGBA{R: uint8(seed), G: uint8(seed >> 8), B: 7, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	if padding := size - buffer.Len(); padding > 0 {
		buffer.Write(bytes.Repeat([]byte{byte(seed)}, padding))
	}
	return buffer.Bytes()
}

// pngHeaderOf is a PNG's signature and IHDR chunk alone, declaring a width×height canvas: all the
// server reads, so a few bytes can claim any size.
func pngHeaderOf(width uint32, height uint32) []byte {
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:4], width)
	binary.BigEndian.PutUint32(header[4:8], height)
	header[8], header[9] = 8, 6
	chunk := append([]byte("IHDR"), header...)
	data := append([]byte("\x89PNG\r\n\x1a\n"), binary.BigEndian.AppendUint32(nil, uint32(len(header)))...)
	data = append(data, chunk...)
	return binary.BigEndian.AppendUint32(data, crc32.ChecksumIEEE(chunk))
}

func (test *mapTest) upload(viewer core.Viewer, data []byte) core.UploadedImage {
	test.t.Helper()
	uploaded, err := test.service.UploadImage(test.context, viewer, data)
	if err != nil {
		test.t.Fatalf("upload: %v", err)
	}
	return uploaded
}

func (test *mapTest) createMap(viewer core.Viewer, name string, imageID uuid.UUID, revealed bool) core.WorldMap {
	test.t.Helper()
	created, err := test.service.CreateMap(test.context, viewer, core.NewMapInput{
		MapInput: core.MapInput{Name: name}, MapArtInput: core.MapArtInput{ImageID: &imageID}, Revealed: revealed,
	})
	if err != nil {
		test.t.Fatalf("create map %s: %v", name, err)
	}
	return created
}

func (test *mapTest) place(viewer core.Viewer, name string, revealed bool) core.Place {
	test.t.Helper()
	place, err := test.service.CreatePlace(test.context, viewer, core.NewPlaceInput{Revealed: revealed, PlaceInput: core.PlaceInput{Name: name}})
	if err != nil {
		test.t.Fatalf("create place %s: %v", name, err)
	}
	return place
}

func (test *mapTest) pin(placeID uuid.UUID, mapID uuid.UUID, x float64, y float64) core.Place {
	test.t.Helper()
	place, err := test.service.SetPlacePin(test.context, test.dm, placeID, core.PinInput{MapID: mapID, X: x, Y: y})
	if err != nil {
		test.t.Fatalf("pin: %v", err)
	}
	return place
}

func (test *mapTest) imageReadable(viewer core.Viewer, imageID uuid.UUID) bool {
	test.t.Helper()
	_, err := test.service.GetImage(test.context, viewer, imageID)
	if err != nil && !isKind(err, core.KindNotFound) {
		test.t.Fatalf("read image: %v", err)
	}
	return err == nil
}

func TestUploadsAreSniffedFromTheirBytes(t *testing.T) {
	test := newMapTest(t)
	var jpegBuffer, gifBuffer bytes.Buffer
	if err := jpeg.Encode(&jpegBuffer, image.NewRGBA(image.Rect(0, 0, 21, 13)), nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifBuffer, image.NewPaletted(image.Rect(0, 0, 5, 8), []color.Color{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	eldari, err := os.ReadFile("../../web/public/eldari.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name          string
		data          []byte
		contentType   string
		width, height int64
	}{
		{"a PNG", pngOf(t, 40, 30, 1, 0), "image/png", 40, 30},
		{"a JPEG", jpegBuffer.Bytes(), "image/jpeg", 21, 13},
		{"a GIF", gifBuffer.Bytes(), "image/gif", 5, 8},
		{"a WebP", eldari, "image/webp", 1920, 1080},
	} {
		uploaded := test.upload(test.dm, check.data)
		if uploaded.ContentType != check.contentType || uploaded.Width != check.width || uploaded.Height != check.height || uploaded.ByteSize != int64(len(check.data)) {
			t.Errorf("%s uploaded as %+v", check.name, uploaded)
		}
	}

	for name, data := range map[string][]byte{
		"an SVG":            []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>alert(1)</script></svg>`),
		"a text file":       []byte("this is a PNG, honest"),
		"a truncated PNG":   pngOf(t, 4, 4, 2, 0)[:20],
		"nothing":           {},
		"a RIFF of no WebP": append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 30)...),
	} {
		if _, err := test.service.UploadImage(test.context, test.dm, data); !isKind(err, core.KindInvalid) {
			t.Errorf("%s: err = %v, want refused", name, err)
		}
	}
	if _, err := test.service.UploadImage(test.context, test.dm, pngOf(t, 4, 4, 3, core.ImageMaxBytes+1)); !isKind(err, core.KindInvalid) {
		t.Errorf("an image over 5 MB: err = %v, want refused", err)
	}
	// A small file declaring a canvas no phone can decode never reaches the players' browsers.
	for name, data := range map[string][]byte{
		"40000×40000":                   pngHeaderOf(40000, 40000),
		"one pixel past the side limit": pngHeaderOf(core.ImageMaxSide+1, 1),
		"over the pixel limit":          pngHeaderOf(core.ImageMaxSide, core.ImageMaxPixels/core.ImageMaxSide+1),
	} {
		if _, err := test.service.UploadImage(test.context, test.dm, data); !isKind(err, core.KindInvalid) || !strings.Contains(err.Error(), "megapixels") {
			t.Errorf("%s: err = %v, want the size refused", name, err)
		}
	}
	if largest := test.upload(test.dm, pngHeaderOf(core.ImageMaxSide, core.ImageMaxPixels/core.ImageMaxSide)); largest.Width != core.ImageMaxSide {
		t.Errorf("the largest allowed image uploaded as %+v", largest)
	}
	for name, viewer := range map[string]core.Viewer{"a player": test.player, "the DM viewing as player": test.dmAsPlayer, "a pending member": test.pending} {
		if _, err := test.service.UploadImage(test.context, viewer, pngOf(t, 4, 4, 4, 0)); err == nil {
			t.Errorf("%s uploaded an image", name)
		}
	}

	// The same bytes again are the stored copy, in this campaign; another campaign stores its own.
	first := test.upload(test.dm, pngOf(t, 4, 4, 5, 0))
	if again := test.upload(test.dm, pngOf(t, 4, 4, 5, 0)); again.ID != first.ID {
		t.Fatalf("a repeated upload made a second image: %v, %v", again.ID, first.ID)
	}
	if other := test.upload(test.otherDm, pngOf(t, 4, 4, 5, 0)); other.ID == first.ID {
		t.Fatal("two campaigns share one image")
	}
	if test.imageReadable(test.otherDm, first.ID) {
		t.Fatal("another campaign's DM read this campaign's image")
	}
}

func TestTheImageBudgetHoldsUnderConcurrentUploads(t *testing.T) {
	test := newMapTest(t)
	const attempts = core.ImageBudgetBytes/core.ImageMaxBytes + 2
	var waitGroup sync.WaitGroup
	results := make([]error, attempts)
	for index := range attempts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, results[index] = test.service.UploadImage(test.context, test.dm, pngOf(t, 4, 4, index+10, core.ImageMaxBytes))
		}()
	}
	waitGroup.Wait()
	stored, refused := 0, 0
	var refusal error
	for _, err := range results {
		switch {
		case err == nil:
			stored++
		case isKind(err, core.KindConflict):
			refused++
			refusal = err
		default:
			t.Fatalf("upload: %v", err)
		}
	}
	if stored != core.ImageBudgetBytes/core.ImageMaxBytes || refused != 2 {
		t.Fatalf("%d stored, %d refused; want the budget's worth stored", stored, refused)
	}
	if refusal.Error() != "This campaign's images use 50 MB of 50 MB. Replace or remove a map or portrait first." {
		t.Fatalf("refusal = %q", refusal.Error())
	}
	// The budget is per campaign.
	test.upload(test.otherDm, pngOf(t, 4, 4, 99, core.ImageMaxBytes))
}

func TestImagesNothingUsesAreDeleted(t *testing.T) {
	test := newMapTest(t)
	first := test.upload(test.dm, pngOf(t, 40, 30, 1, 0))
	worldMap := test.createMap(test.dm, "Mainland", first.ID, true)
	sitter, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Brannoc"}})
	if err != nil {
		t.Fatal(err)
	}
	// One image as map art and a portrait: replacing the art keeps it for the portrait.
	if _, err := test.service.SetNpcPortrait(test.context, test.dm, sitter.ID, core.NpcPortraitInput{ImageID: &first.ID}); err != nil {
		t.Fatal(err)
	}
	second := test.upload(test.dm, pngOf(t, 80, 60, 2, 0))
	if _, err := test.service.ReplaceMapArt(test.context, test.dm, worldMap.ID, core.MapArtInput{ImageID: &second.ID}); err != nil {
		t.Fatal(err)
	}
	if !test.imageReadable(test.dm, first.ID) {
		t.Fatal("the portrait's image was deleted with the old map art")
	}
	third := test.upload(test.dm, pngOf(t, 8, 8, 3, 0))
	if _, err := test.service.SetNpcPortrait(test.context, test.dm, sitter.ID, core.NpcPortraitInput{ImageID: &third.ID}); err != nil {
		t.Fatal(err)
	}
	if test.imageReadable(test.dm, first.ID) {
		t.Fatal("an image nothing uses was kept")
	}
	if err := test.service.DeleteNpc(test.context, test.dm, sitter.ID); err != nil {
		t.Fatal(err)
	}
	if test.imageReadable(test.dm, third.ID) {
		t.Fatal("a deleted NPC's portrait was kept")
	}
	if err := test.service.DeleteMap(test.context, test.dm, worldMap.ID); err != nil {
		t.Fatal(err)
	}
	if test.imageReadable(test.dm, second.ID) {
		t.Fatal("a deleted map's art was kept")
	}

	// An upload nothing took up within the hour is swept by the next upload, a fresh one is not.
	stale := test.upload(test.dm, pngOf(t, 8, 8, 4, 0))
	fresh := test.upload(test.dm, pngOf(t, 8, 8, 5, 0))
	if _, err := test.database.Pool.Exec(test.context, `UPDATE image SET created_at = now() - interval '2 hours' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}
	test.upload(test.dm, pngOf(t, 8, 8, 6, 0))
	if test.imageReadable(test.dm, stale.ID) || !test.imageReadable(test.dm, fresh.ID) {
		t.Fatal("the sweep kept a stale upload or took a fresh one")
	}

	// The same bytes again hand out the stored upload with a full hour again, so the next upload's sweep
	// can't take the image a form was just given.
	if _, err := test.database.Pool.Exec(test.context, `UPDATE image SET created_at = now() - interval '59 minutes' WHERE id = $1`, fresh.ID); err != nil {
		t.Fatal(err)
	}
	again := test.upload(test.dm, pngOf(t, 8, 8, 5, 0))
	var renewed bool
	if err := test.database.Pool.QueryRow(test.context, `SELECT created_at > now() - interval '1 minute' FROM image WHERE id = $1`, fresh.ID).Scan(&renewed); err != nil {
		t.Fatal(err)
	}
	if again.ID != fresh.ID || !renewed {
		t.Fatalf("the same bytes again = %s (stored %s), hour renewed %v", again.ID, fresh.ID, renewed)
	}
}

func TestPlayersNeverSeeHiddenMapsPinsOrPortraits(t *testing.T) {
	test := newMapTest(t)
	shownArt := test.upload(test.dm, pngOf(t, 40, 30, 1, 0))
	hiddenArt := test.upload(test.dm, pngOf(t, 40, 30, 2, 0))
	shown := test.createMap(test.dm, "Mainland", shownArt.ID, true)
	hidden := test.createMap(test.dm, "map-canary", hiddenArt.ID, false)
	visiblePlace := test.place(test.dm, "Docks", true)
	hiddenPlace := test.place(test.dm, "place-canary", false)
	onHiddenMap := test.place(test.dm, "Ridge", true)
	test.pin(visiblePlace.ID, shown.ID, 0.25, 0.75)
	test.pin(hiddenPlace.ID, shown.ID, 0.5, 0.5)
	test.pin(onHiddenMap.ID, hidden.ID, 0.1, 0.2)
	shownPortrait := test.upload(test.dm, pngOf(t, 8, 8, 3, 0))
	hiddenPortrait := test.upload(test.dm, pngOf(t, 8, 8, 4, 0))
	unused := test.upload(test.dm, pngOf(t, 8, 8, 5, 0))
	for _, sitter := range []struct {
		name     string
		revealed bool
		imageID  uuid.UUID
	}{{"Lady Vex", true, shownPortrait.ID}, {"Edric", false, hiddenPortrait.ID}} {
		npc, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{Revealed: sitter.revealed, NpcInput: core.NpcInput{Name: sitter.name}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := test.service.SetNpcPortrait(test.context, test.dm, npc.ID, core.NpcPortraitInput{ImageID: &sitter.imageID}); err != nil {
			t.Fatal(err)
		}
	}

	for name, viewer := range map[string]core.Viewer{"a player": test.player, "the DM viewing as player": test.dmAsPlayer} {
		maps, err := test.service.ListMaps(test.context, viewer)
		if err != nil || len(maps.Maps) != 1 || maps.Maps[0].ID != shown.ID {
			t.Fatalf("%s lists maps %+v (%v)", name, maps, err)
		}
		_, hiddenErr := test.service.GetMap(test.context, viewer, hidden.ID)
		_, missingErr := test.service.GetMap(test.context, viewer, uuid.New())
		if !isKind(hiddenErr, core.KindNotFound) || hiddenErr.Error() != missingErr.Error() {
			t.Fatalf("%s reading a hidden map: %v", name, hiddenErr)
		}
		page, err := test.service.GetMap(test.context, viewer, shown.ID)
		if err != nil || len(page.Pins) != 1 || page.Pins[0].PlaceID != visiblePlace.ID || page.Pins[0].X != 0.25 || page.Pins[0].Y != 0.75 {
			t.Fatalf("%s sees the pins %+v (%v)", name, page.Pins, err)
		}
		ridge, err := test.service.GetPlace(test.context, viewer, onHiddenMap.ID)
		if err != nil || ridge.Pin != nil {
			t.Fatalf("%s sees a pin on a hidden map: %+v (%v)", name, ridge.Pin, err)
		}
		places, err := test.service.ListPlaces(test.context, viewer, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, place := range places.Places {
			if place.ID == hiddenPlace.ID || (place.Pin != nil && place.Pin.MapID == hidden.ID) {
				t.Fatalf("%s lists a hidden place or a pin on a hidden map: %+v", name, place)
			}
		}
		for imageID, want := range map[uuid.UUID]bool{shownArt.ID: true, hiddenArt.ID: false, shownPortrait.ID: true, hiddenPortrait.ID: false, unused.ID: false} {
			if readable := test.imageReadable(viewer, imageID); readable != want {
				t.Errorf("%s reads image %v: %v, want %v", name, imageID, readable, want)
			}
		}
	}

	// The DM sees everything; revealing the map shows it and its pins to players.
	maps, err := test.service.ListMaps(test.context, test.dm)
	if err != nil || len(maps.Maps) != 2 || maps.Maps[1].Dm == nil || maps.Maps[1].Dm.Revealed {
		t.Fatalf("the DM lists maps %+v (%v)", maps, err)
	}
	if page, err := test.service.GetMap(test.context, test.dm, shown.ID); err != nil || len(page.Pins) != 2 {
		t.Fatalf("the DM sees the pins %+v (%v)", page.Pins, err)
	}
	for _, imageID := range []uuid.UUID{hiddenArt.ID, hiddenPortrait.ID, unused.ID} {
		if !test.imageReadable(test.dm, imageID) {
			t.Fatalf("the DM can't read image %v", imageID)
		}
	}
	if _, err := test.service.SetMapRevealed(test.context, test.dm, hidden.ID, true); err != nil {
		t.Fatal(err)
	}
	ridge, err := test.service.GetPlace(test.context, test.player, onHiddenMap.ID)
	if err != nil || ridge.Pin == nil || ridge.Pin.MapID != hidden.ID || ridge.Pin.MapName != "map-canary" {
		t.Fatalf("a revealed map's pin = %+v (%v)", ridge.Pin, err)
	}
	if !test.imageReadable(test.player, hiddenArt.ID) {
		t.Fatal("a revealed map's art stays unreadable")
	}
	// Nothing about maps, pins or images reaches the timeline.
	timeline, err := test.service.ListTimeline(test.context, test.dm, core.TimelineRangeInput{FromDay: 0, ToDay: 10})
	if err != nil || len(timeline.Entries) != 0 {
		t.Fatalf("the timeline has %d entries (%v); want none", len(timeline.Entries), err)
	}
}

func TestMapsAndPins(t *testing.T) {
	test := newMapTest(t)
	small := test.upload(test.dm, pngOf(t, 40, 30, 1, 0))
	first := test.createMap(test.dm, "Eldari", small.ID, true)
	if first.Width != 40 || first.Height != 30 || first.ImageID == nil || *first.ImageID != small.ID || first.IsMainland {
		t.Fatalf("created %+v", first)
	}
	docks := test.place(test.dm, "Docks", true)
	pinned := test.pin(docks.ID, first.ID, 0.25, 0.75)
	if pinned.Pin == nil || pinned.Pin.MapID != first.ID || pinned.Pin.MapName != "Eldari" || pinned.Pin.X != 0.25 || pinned.Pin.Y != 0.75 {
		t.Fatalf("pin = %+v", pinned.Pin)
	}

	// A larger copy of the art keeps the pins where they were.
	large := test.upload(test.dm, pngOf(t, 400, 300, 2, 0))
	replaced, err := test.service.ReplaceMapArt(test.context, test.dm, first.ID, core.MapArtInput{ImageID: &large.ID})
	if err != nil || replaced.Width != 400 || replaced.Height != 300 || len(replaced.Pins) != 1 || replaced.Pins[0].X != 0.25 || replaced.Pins[0].Y != 0.75 {
		t.Fatalf("after the new art: %+v (%v)", replaced, err)
	}

	// An https address with its size is art too; the bundled sign-in art and other paths are not (C6).
	external, err := test.service.CreateMap(test.context, test.dm, core.NewMapInput{
		MapInput:    core.MapInput{Name: "Far Isles"},
		MapArtInput: core.MapArtInput{ExternalURL: "https://example.com/isles.png", Width: 1200, Height: 800},
		Revealed:    true,
		Mainland:    true,
	})
	if err != nil || external.ExternalURL != "https://example.com/isles.png" || external.ImageID != nil || !external.IsMainland {
		t.Fatalf("external map = %+v (%v)", external, err)
	}
	foreignImageID := test.upload(test.otherDm, pngOf(t, 4, 4, 3, 0)).ID
	for name, art := range map[string]core.MapArtInput{
		"the bundled Eldari":                 {ExternalURL: "/eldari.webp", Width: 1920, Height: 1080},
		"the bundled Eldari over https":      {ExternalURL: "https://colony.example/eldari.webp", Width: 1920, Height: 1080},
		"this site with a port and capitals": {ExternalURL: "https://COLONY.example.:8443/./eldari.webp?x", Width: 1920, Height: 1080},
		// Browsers map each of these hosts to this site before connecting.
		"this site in fullwidth letters":        {ExternalURL: "https://ｃｏｌｏｎｙ.example/eldari.webp", Width: 1920, Height: 1080},
		"this site with an escaped soft hyphen": {ExternalURL: "https://colo%C2%ADny.example/eldari.webp", Width: 1920, Height: 1080},
		"this site with a soft hyphen":          {ExternalURL: "https://colo\u00adny.example/eldari.webp", Width: 1920, Height: 1080},
		"this site with an ideographic stop":    {ExternalURL: "https://colony.example%E3%80%82/eldari.webp", Width: 1920, Height: 1080},
		"plain http":                            {ExternalURL: "http://example.com/a.png", Width: 10, Height: 10},
		"no size":                               {ExternalURL: "https://example.com/a.png"},
		"both":                                  {ImageID: &large.ID, ExternalURL: "https://example.com/a.png", Width: 10, Height: 10},
		"neither":                               {},
		"another campaign's":                    {ImageID: &foreignImageID},
	} {
		if _, err := test.service.CreateMap(test.context, test.dm, core.NewMapInput{MapInput: core.MapInput{Name: "Bad"}, MapArtInput: art}); !isKind(err, core.KindInvalid) {
			t.Errorf("%s: err = %v, want a field error", name, err)
		}
	}
	if _, err := test.service.CreateMap(test.context, test.dm, core.NewMapInput{MapInput: core.MapInput{Name: " "}, MapArtInput: core.MapArtInput{ImageID: &large.ID}}); !isKind(err, core.KindInvalid) {
		t.Errorf("a blank name: err = %v", err)
	}

	// One Mainland at most: setting it moves it, clearing it leaves none.
	if moved, err := test.service.SetMapMainland(test.context, test.dm, first.ID, true); err != nil || !moved.IsMainland {
		t.Fatalf("set the Mainland: %+v (%v)", moved, err)
	}
	if page, err := test.service.GetMap(test.context, test.dm, external.ID); err != nil || page.IsMainland {
		t.Fatalf("two Mainlands: %+v (%v)", page, err)
	}
	if cleared, err := test.service.SetMapMainland(test.context, test.dm, first.ID, false); err != nil || cleared.IsMainland {
		t.Fatalf("clear the Mainland: %+v (%v)", cleared, err)
	}

	// Rename changes only the name; reorder takes every map once.
	renamed, err := test.service.RenameMap(test.context, test.dm, first.ID, core.MapInput{Name: "The Mainland"})
	if err != nil || renamed.Name != "The Mainland" || !renamed.Dm.Revealed || len(renamed.Pins) != 1 {
		t.Fatalf("renamed = %+v (%v)", renamed, err)
	}
	ordered, err := test.service.ReorderMaps(test.context, test.dm, core.MapOrderInput{MapIDs: []uuid.UUID{external.ID, first.ID}})
	if err != nil || len(ordered.Maps) != 2 || ordered.Maps[0].ID != external.ID {
		t.Fatalf("reordered = %+v (%v)", ordered, err)
	}
	for name, mapIDs := range map[string][]uuid.UUID{
		"a missing map":       {external.ID},
		"a repeated map":      {external.ID, external.ID},
		"another campaign's":  {external.ID, first.ID, test.createMap(test.otherDm, "Ridge", test.upload(test.otherDm, pngOf(t, 4, 4, 4, 0)).ID, true).ID},
		"an unknown map's ID": {external.ID, uuid.New()},
	} {
		if _, err := test.service.ReorderMaps(test.context, test.dm, core.MapOrderInput{MapIDs: mapIDs}); !isKind(err, core.KindConflict) {
			t.Errorf("reorder with %s: err = %v", name, err)
		}
	}

	// Pins are fractions of the art, on a map of this campaign.
	otherMap, err := test.service.ListMaps(test.context, test.otherDm)
	if err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]core.PinInput{
		"x past the edge":    {MapID: first.ID, X: 1.01, Y: 0.5},
		"y below zero":       {MapID: first.ID, X: 0.5, Y: -0.1},
		"another campaign's": {MapID: otherMap.Maps[0].ID, X: 0.5, Y: 0.5},
		"an unknown map":     {MapID: uuid.New(), X: 0.5, Y: 0.5},
	} {
		if _, err := test.service.SetPlacePin(test.context, test.dm, docks.ID, input); !isKind(err, core.KindInvalid) {
			t.Errorf("a pin with %s: err = %v", name, err)
		}
	}
	if _, err := test.service.SetPlacePin(test.context, test.player, docks.ID, core.PinInput{MapID: first.ID, X: 0.1, Y: 0.1}); err == nil {
		t.Error("a player moved a pin")
	}
	if _, err := test.service.SetPlacePin(test.context, test.otherDm, docks.ID, core.PinInput{MapID: otherMap.Maps[0].ID, X: 0.1, Y: 0.1}); !isKind(err, core.KindNotFound) {
		t.Errorf("another campaign's DM pinned this campaign's place: %v", err)
	}
	moved := test.pin(docks.ID, external.ID, 1, 0)
	if moved.Pin == nil || moved.Pin.MapID != external.ID || moved.Pin.X != 1 || moved.Pin.Y != 0 {
		t.Fatalf("moved pin = %+v", moved.Pin)
	}
	if cleared, err := test.service.ClearPlacePin(test.context, test.dm, docks.ID); err != nil || cleared.Pin != nil {
		t.Fatalf("cleared pin = %+v (%v)", cleared.Pin, err)
	}

	// Deleting a map unpins its places and keeps them.
	test.pin(docks.ID, first.ID, 0.5, 0.5)
	if err := test.service.DeleteMap(test.context, test.dm, first.ID); err != nil {
		t.Fatal(err)
	}
	if place, err := test.service.GetPlace(test.context, test.dm, docks.ID); err != nil || place.Pin != nil {
		t.Fatalf("the place of a deleted map = %+v (%v)", place, err)
	}
	if err := test.service.DeleteMap(test.context, test.dm, first.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("deleting a deleted map: %v", err)
	}
	if err := test.service.DeleteMap(test.context, test.otherDm, external.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("another campaign's DM deleted a map: %v", err)
	}
	for name, viewer := range map[string]core.Viewer{"a player": test.player, "the DM viewing as player": test.dmAsPlayer} {
		if _, err := test.service.SetMapRevealed(test.context, viewer, external.ID, false); err == nil {
			t.Errorf("%s hid a map", name)
		}
		if _, err := test.service.CreateMap(test.context, viewer, core.NewMapInput{MapInput: core.MapInput{Name: "Mine"}, MapArtInput: core.MapArtInput{ImageID: &large.ID}}); err == nil {
			t.Errorf("%s created a map", name)
		}
	}
}

func TestPortraits(t *testing.T) {
	test := newMapTest(t)
	npc, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Lady Vex"}})
	if err != nil {
		t.Fatal(err)
	}
	portrait := test.upload(test.dm, pngOf(t, 8, 8, 1, 0))
	withPortrait, err := test.service.SetNpcPortrait(test.context, test.dm, npc.ID, core.NpcPortraitInput{ImageID: &portrait.ID})
	if err != nil || withPortrait.PortraitImageID == nil || *withPortrait.PortraitImageID != portrait.ID {
		t.Fatalf("portrait = %+v (%v)", withPortrait.PortraitImageID, err)
	}
	if seen, err := test.service.GetNpc(test.context, test.player, npc.ID); err != nil || seen.PortraitImageID == nil {
		t.Fatalf("the player's NPC = %+v (%v)", seen, err)
	}
	foreign := test.upload(test.otherDm, pngOf(t, 8, 8, 2, 0))
	if _, err := test.service.SetNpcPortrait(test.context, test.dm, npc.ID, core.NpcPortraitInput{ImageID: &foreign.ID}); !isKind(err, core.KindInvalid) {
		t.Fatalf("another campaign's image as a portrait: %v", err)
	}
	if _, err := test.service.SetNpcPortrait(test.context, test.otherDm, npc.ID, core.NpcPortraitInput{ImageID: &foreign.ID}); !isKind(err, core.KindNotFound) {
		t.Fatalf("another campaign's DM set this campaign's portrait: %v", err)
	}
	if _, err := test.service.SetNpcPortrait(test.context, test.player, npc.ID, core.NpcPortraitInput{}); err == nil {
		t.Fatal("a player cleared a portrait")
	}
	cleared, err := test.service.SetNpcPortrait(test.context, test.dm, npc.ID, core.NpcPortraitInput{})
	if err != nil || cleared.PortraitImageID != nil {
		t.Fatalf("cleared portrait = %+v (%v)", cleared.PortraitImageID, err)
	}
	if test.imageReadable(test.dm, portrait.ID) {
		t.Fatal("a removed portrait's image was kept")
	}
}

func TestHexPainting(t *testing.T) {
	test := newMapTest(t)
	paint := func(viewer core.Viewer, cells ...core.PaintedHex) error {
		return test.service.PaintHexes(test.context, viewer, core.PaintHexesInput{Cells: cells})
	}
	hexesOf := func(viewer core.Viewer) map[core.HexCoordinate]core.Hex {
		t.Helper()
		list, err := test.service.ListHexes(test.context, viewer)
		if err != nil {
			t.Fatal(err)
		}
		byCoordinate := map[core.HexCoordinate]core.Hex{}
		for _, hex := range list.Hexes {
			byCoordinate[core.HexCoordinate{Q: hex.Q, R: hex.R}] = hex
		}
		return byCoordinate
	}
	// Any active member paints; the last terrain named for a cell wins.
	if err := paint(test.player, core.PaintedHex{Q: 0, R: 0, Terrain: core.TerrainForest}, core.PaintedHex{Q: 1, R: -1, Terrain: core.TerrainWater},
		core.PaintedHex{Q: 1, R: -1, Terrain: core.TerrainSwamp}); err != nil {
		t.Fatal(err)
	}
	if err := test.service.SetHexLabel(test.context, test.dmAsPlayer, core.HexCoordinate{Q: 0, R: 0}, core.HexLabelInput{Label: "  Wolf Den  "}); err != nil {
		t.Fatal(err)
	}
	// Repainting keeps the label.
	if err := paint(test.dm, core.PaintedHex{Q: 0, R: 0, Terrain: core.TerrainMountain}); err != nil {
		t.Fatal(err)
	}
	hexes := hexesOf(test.player)
	if len(hexes) != 2 || hexes[core.HexCoordinate{}] != (core.Hex{Terrain: core.TerrainMountain, Label: "Wolf Den"}) ||
		hexes[core.HexCoordinate{Q: 1, R: -1}].Terrain != core.TerrainSwamp {
		t.Fatalf("hexes = %+v", hexes)
	}
	if len(hexesOf(test.otherDm)) != 0 {
		t.Fatal("another campaign sees this campaign's hexes")
	}
	// Clearing a label keeps the terrain; an uncharted hex has nothing to label.
	if err := test.service.SetHexLabel(test.context, test.player, core.HexCoordinate{}, core.HexLabelInput{Label: ""}); err != nil {
		t.Fatal(err)
	}
	if hex := hexesOf(test.player)[core.HexCoordinate{}]; hex.Label != "" || hex.Terrain != core.TerrainMountain {
		t.Fatalf("after clearing the label: %+v", hex)
	}
	if err := test.service.SetHexLabel(test.context, test.player, core.HexCoordinate{Q: 5, R: 5}, core.HexLabelInput{Label: "Nowhere"}); !isKind(err, core.KindNotFound) {
		t.Fatalf("labelling an uncharted hex: %v", err)
	}
	if err := test.service.SetHexLabel(test.context, test.player, core.HexCoordinate{}, core.HexLabelInput{Label: string(bytes.Repeat([]byte("x"), 41))}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a 41-character label: %v", err)
	}
	// Erasing takes terrain and label together.
	if err := test.service.SetHexLabel(test.context, test.player, core.HexCoordinate{Q: 1, R: -1}, core.HexLabelInput{Label: "Bog"}); err != nil {
		t.Fatal(err)
	}
	if err := test.service.EraseHexes(test.context, test.player, core.EraseHexesInput{Cells: []core.HexCoordinate{{Q: 1, R: -1}, {Q: 9, R: 9}}}); err != nil {
		t.Fatal(err)
	}
	if err := paint(test.player, core.PaintedHex{Q: 1, R: -1, Terrain: core.TerrainDesert}); err != nil {
		t.Fatal(err)
	}
	if hex := hexesOf(test.player)[core.HexCoordinate{Q: 1, R: -1}]; hex.Label != "" || hex.Terrain != core.TerrainDesert {
		t.Fatalf("an erased hex kept its label: %+v", hex)
	}

	// Only bad input is refused: coordinates past ±100,000, an unknown terrain, too many or no cells.
	tooMany := make([]core.PaintedHex, core.HexCellsPerCall+1)
	for index := range tooMany {
		tooMany[index] = core.PaintedHex{Q: int64(index), R: 0, Terrain: core.TerrainPlains}
	}
	for name, cells := range map[string][]core.PaintedHex{
		"q past the guard":  {{Q: core.HexCoordinateLimit + 1, R: 0, Terrain: core.TerrainPlains}},
		"r past the guard":  {{Q: 0, R: -core.HexCoordinateLimit - 1, Terrain: core.TerrainPlains}},
		"a huge q":          {{Q: 1 << 40, R: 0, Terrain: core.TerrainPlains}},
		"an unknown kind":   {{Q: 0, R: 0, Terrain: "LAVA"}},
		"no cells":          {},
		"over one stroke's": tooMany,
	} {
		if err := paint(test.player, cells...); !isKind(err, core.KindInvalid) {
			t.Errorf("painting %s: err = %v", name, err)
		}
	}
	if err := paint(test.player, tooMany[:core.HexCellsPerCall]...); err != nil {
		t.Fatalf("a full stroke: %v", err)
	}
	if err := paint(test.player, core.PaintedHex{Q: core.HexCoordinateLimit, R: -core.HexCoordinateLimit, Terrain: core.TerrainTundra}); err != nil {
		t.Fatalf("painting at the guard: %v", err)
	}
	if err := test.service.EraseHexes(test.context, test.player, core.EraseHexesInput{Cells: []core.HexCoordinate{{Q: 0, R: core.HexCoordinateLimit + 1}}}); !isKind(err, core.KindInvalid) {
		t.Errorf("erasing past the guard: %v", err)
	}
	if err := test.service.SetHexLabel(test.context, test.player, core.HexCoordinate{Q: -core.HexCoordinateLimit - 1}, core.HexLabelInput{Label: "x"}); !isKind(err, core.KindInvalid) {
		t.Errorf("labelling past the guard: %v", err)
	}
	if err := paint(test.pending, core.PaintedHex{Q: 7, R: 7, Terrain: core.TerrainPlains}); err == nil {
		t.Error("a pending member painted")
	}
	// Nothing about the hex map reaches the timeline.
	timeline, err := test.service.ListTimeline(test.context, test.dm, core.TimelineRangeInput{FromDay: 0, ToDay: 10})
	if err != nil || len(timeline.Entries) != 0 {
		t.Fatalf("the timeline has %d entries (%v); want none", len(timeline.Entries), err)
	}
}

func TestTheHexLimitHoldsUnderConcurrentStrokes(t *testing.T) {
	test := newMapTest(t)
	const room = 30
	if _, err := test.database.Pool.Exec(test.context, `INSERT INTO hex (campaign_id, q, r, terrain)
		SELECT $1, cell, 0, 'PLAINS' FROM generate_series(1, $2::integer) AS cell`, test.dm.CampaignID(), core.HexLimit-room); err != nil {
		t.Fatal(err)
	}
	// Repainting charted hexes is never refused, even at the limit.
	if err := test.service.PaintHexes(test.context, test.player, core.PaintHexesInput{Cells: []core.PaintedHex{{Q: 1, R: 0, Terrain: core.TerrainWater}}}); err != nil {
		t.Fatal(err)
	}
	const strokes = 6
	var waitGroup sync.WaitGroup
	results := make([]error, strokes)
	for stroke := range strokes {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			cells := make([]core.PaintedHex, 10)
			for index := range cells {
				cells[index] = core.PaintedHex{Q: int64(index), R: int64(stroke + 1), Terrain: core.TerrainForest}
			}
			results[stroke] = test.service.PaintHexes(test.context, test.player, core.PaintHexesInput{Cells: cells})
		}()
	}
	waitGroup.Wait()
	painted := 0
	for _, err := range results {
		switch {
		case err == nil:
			painted++
		case isKind(err, core.KindConflict):
		default:
			t.Fatalf("stroke: %v", err)
		}
	}
	list, err := test.service.ListHexes(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	if painted != room/10 || len(list.Hexes) != core.HexLimit {
		t.Fatalf("%d strokes painted, %d hexes; want %d and the limit", painted, len(list.Hexes), room/10)
	}
	err = test.service.PaintHexes(test.context, test.player, core.PaintHexesInput{Cells: []core.PaintedHex{{Q: -1, R: -1, Terrain: core.TerrainForest}}})
	if !isKind(err, core.KindConflict) || err.Error() != "The map has 50,000 charted hexes and holds at most 50,000. Unchart some before charting more." {
		t.Fatalf("past the limit: %v", err)
	}
	// The limit is per campaign.
	if err := test.service.PaintHexes(test.context, test.otherDm, core.PaintHexesInput{Cells: []core.PaintedHex{{Q: -1, R: -1, Terrain: core.TerrainForest}}}); err != nil {
		t.Fatalf("another campaign: %v", err)
	}
}

func TestOneMainlandWhenMapsTakeItAtOnce(t *testing.T) {
	test := newMapTest(t)
	art := test.upload(test.dm, pngOf(t, 4, 4, 1, 0))
	mapIDs := make([]uuid.UUID, 5)
	for index := range mapIDs {
		mapIDs[index] = test.createMap(test.dm, "Map", art.ID, true).ID
	}
	var waitGroup sync.WaitGroup
	results := make([]error, len(mapIDs))
	for index, mapID := range mapIDs {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, results[index] = test.service.SetMapMainland(test.context, test.dm, mapID, true)
		}()
	}
	waitGroup.Wait()
	for _, err := range results {
		if err != nil {
			t.Fatalf("set the Mainland: %v", err)
		}
	}
	maps, err := test.service.ListMaps(test.context, test.player)
	if err != nil {
		t.Fatal(err)
	}
	mainlands := 0
	for _, worldMap := range maps.Maps {
		if worldMap.IsMainland {
			mainlands++
		}
	}
	if mainlands != 1 {
		t.Fatalf("%d Mainlands, want 1", mainlands)
	}
}

// The colony board's faint backdrop is the Mainland of the viewer's own map list (C6: never the bundled
// Eldari), so a hidden Mainland's art reaches no player and no DM viewing as player that way.
func TestAHiddenMainlandIsNoPlayersBackdrop(t *testing.T) {
	test := newMapTest(t)
	art := test.upload(test.dm, pngOf(t, 40, 30, 1, 0))
	mainland := test.createMap(test.dm, "Mainland", art.ID, false)
	if updated, err := test.service.SetMapMainland(test.context, test.dm, mainland.ID, true); err != nil || !updated.IsMainland {
		t.Fatalf("set the Mainland: %+v (%v)", updated, err)
	}
	backdrop := func(viewer core.Viewer) *uuid.UUID {
		t.Helper()
		list, err := test.service.ListMaps(test.context, viewer)
		if err != nil {
			t.Fatal(err)
		}
		for _, worldMap := range list.Maps {
			if worldMap.IsMainland {
				return worldMap.ImageID
			}
		}
		return nil
	}
	players := map[string]core.Viewer{"a player": test.player, "the DM viewing as player": test.dmAsPlayer}
	for name, viewer := range players {
		if imageID := backdrop(viewer); imageID != nil || test.imageReadable(viewer, art.ID) {
			t.Errorf("%s got the hidden Mainland's art (%v)", name, imageID)
		}
	}
	if imageID := backdrop(test.dm); imageID == nil || *imageID != art.ID {
		t.Fatalf("the DM's backdrop = %v, want the Mainland's art", imageID)
	}
	if _, err := test.service.SetMapRevealed(test.context, test.dm, mainland.ID, true); err != nil {
		t.Fatal(err)
	}
	for name, viewer := range players {
		if imageID := backdrop(viewer); imageID == nil || *imageID != art.ID || !test.imageReadable(viewer, art.ID) {
			t.Errorf("%s has no backdrop once the Mainland is revealed (%v)", name, imageID)
		}
	}
}

// A map delete holds the colony lock from clearing its pins to deleting the row, so a pin waits for it
// and then finds the map gone: no pin lands in between, where ON DELETE SET NULL would orphan it.
func TestAPinWaitsForAMapBeingDeleted(t *testing.T) {
	test := newMapTest(t)
	doomed := test.createMap(test.dm, "Doomed", test.upload(test.dm, pngOf(t, 40, 30, 1, 0)).ID, true)
	place := test.place(test.dm, "Docks", true)
	campaignID := test.dm.CampaignID()
	deleting, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = deleting.Rollback(test.context) }()
	for _, statement := range []string{
		`SELECT 1 FROM colony WHERE campaign_id = $1 FOR UPDATE`,
		`UPDATE place SET map_id = NULL, pin_x = NULL, pin_y = NULL WHERE campaign_id = $1 AND map_id = $2`,
	} {
		arguments := []any{campaignID}
		if strings.Contains(statement, "$2") {
			arguments = append(arguments, doomed.ID)
		}
		if _, err := deleting.Exec(test.context, statement, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	pinned := make(chan error, 1)
	go func() {
		_, err := test.service.SetPlacePin(test.context, test.dm, place.ID, core.PinInput{MapID: doomed.ID, X: 0.5, Y: 0.5})
		pinned <- err
	}()
	select {
	case err := <-pinned:
		t.Fatalf("the pin didn't wait for the map's delete: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := deleting.Exec(test.context, `DELETE FROM map WHERE campaign_id = $1 AND id = $2`, campaignID, doomed.ID); err != nil {
		t.Fatal(err)
	}
	if err := deleting.Commit(test.context); err != nil {
		t.Fatal(err)
	}
	if err := <-pinned; !isKind(err, core.KindInvalid) {
		t.Fatalf("pinning on a deleted map: err = %v, want the missing map's field error", err)
	}
	var unpinned bool
	if err := test.database.Pool.QueryRow(test.context, `SELECT map_id IS NULL AND pin_x IS NULL FROM place WHERE id = $1`, place.ID).Scan(&unpinned); err != nil || !unpinned {
		t.Fatalf("the place kept an orphaned pin (%v)", err)
	}
}

// Deleting a campaign takes the colony lock before the campaign row, as every colony write does; the
// other order deadlocks with a write whose insert checks the campaign row while it holds the colony.
func TestDeletingACampaignWaitsForAColonyWrite(t *testing.T) {
	test := newMapTest(t)
	campaignID := test.dm.CampaignID()
	painting, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = painting.Rollback(test.context) }()
	if _, err := painting.Exec(test.context, `SELECT 1 FROM colony WHERE campaign_id = $1 FOR UPDATE`, campaignID); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		deleted <- test.service.DeleteOwnCampaign(test.context, test.dm, core.DeleteCampaignInput{ConfirmSlug: "sample"})
	}()
	select {
	case err := <-deleted:
		t.Fatalf("the delete didn't wait for the colony write: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := painting.Exec(test.context, `INSERT INTO hex (campaign_id, q, r, terrain) VALUES ($1, 0, 0, 'FOREST')`, campaignID); err != nil {
		t.Fatalf("the colony write's insert: %v", err)
	}
	if err := painting.Commit(test.context); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("delete the campaign: %v", err)
	}
}
