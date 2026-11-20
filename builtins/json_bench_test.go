package builtins

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// benchmarkJSONDocument is a room-update payload of the kind a MOO sends to a
// client or web service: nested maps and lists, object references, numbers,
// booleans and prose with non-ASCII text, about 2.7 KB as JSON.
func benchmarkJSONDocument() types.Value {
	str := types.NewStr
	contents := make([]types.Value, 0, 16)
	for i := range 16 {
		contents = append(contents, types.NewMap([][2]types.Value{
			{str("id"), types.NewObj(types.ObjID(1000 + i))},
			{str("name"), str(fmt.Sprintf("a weathered brass lantern %d", i))},
			{str("aliases"), types.NewList([]types.Value{str("lantern"), str("brass"), str(fmt.Sprintf("l%d", i))})},
			{str("weight"), types.NewFloat(1.25 + float64(i))},
			{str("lit"), types.NewBool(i%2 == 0)},
			{str("owner"), types.NewObj(types.ObjID(2 + i%3))},
		}))
	}
	exits := make([]types.Value, 0, 6)
	for i, dir := range []string{"north", "south", "east", "west", "up", "down"} {
		exits = append(exits, types.NewMap([][2]types.Value{
			{str("dir"), str(dir)},
			{str("to"), types.NewObj(types.ObjID(500 + i))},
			{str("locked"), types.NewBool(i == 4)},
		}))
	}
	return types.NewMap([][2]types.Value{
		{str("type"), str("room.update")},
		{str("seq"), types.NewInt(184467)},
		{str("time"), types.NewInt(1763251200)},
		{str("room"), types.NewMap([][2]types.Value{
			{str("id"), types.NewObj(1234)},
			{str("name"), str("The Café at the End of the Pier")},
			{str("description"), str("Salt-stained windows look out over the grey water. A chalkboard reads “today: chowder, 4½ coins”. Gulls argue on the railing outside, and a bell clangs somewhere in the fog.")},
			{str("exits"), types.NewList(exits)},
			{str("contents"), types.NewList(contents)},
			{str("light"), types.NewFloat(0.62)},
			{str("tags"), types.NewList([]types.Value{str("indoors"), str("shop"), str("coastal")})},
		})},
		{str("weather"), types.NewMap([][2]types.Value{
			{str("temp_c"), types.NewFloat(11.5)},
			{str("wind_kph"), types.NewFloat(23.75)},
			{str("fog"), types.NewBool(true)},
		})},
	})
}

func BenchmarkGenerateJSON(b *testing.B) {
	ctx := newTestExecution()
	args := []types.Value{benchmarkJSONDocument()}
	b.ReportAllocs()
	for b.Loop() {
		if res := builtinGenerateJson(ctx, args); !res.IsNormal() {
			b.Fatalf("generate_json: %v", res.Error)
		}
	}
}

func BenchmarkParseJSON(b *testing.B) {
	ctx := newTestExecution()
	encoded := builtinGenerateJson(ctx, []types.Value{benchmarkJSONDocument()})
	if !encoded.IsNormal() {
		b.Fatalf("generate_json: %v", encoded.Error)
	}
	args := []types.Value{encoded.Val}
	b.SetBytes(int64(len(encoded.Val.Str())))
	b.ReportAllocs()
	for b.Loop() {
		if res := builtinParseJson(ctx, args); !res.IsNormal() {
			b.Fatalf("parse_json: %v", res.Error)
		}
	}
}
