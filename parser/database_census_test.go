package parser_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/types"
)

type censusIdentity struct {
	ObjectID  types.ObjID `json:"object_id"`
	VerbIndex int         `json:"verb_index"`
	VerbName  string      `json:"verb_name"`
}

type censusFailure struct {
	censusIdentity
	Stage  string `json:"stage"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail"`
}

type censusFamily struct {
	Stage    string           `json:"stage"`
	Detail   string           `json:"detail"`
	Count    int              `json:"count"`
	Examples []censusIdentity `json:"examples"`
}

type censusCounts struct {
	Attempted int `json:"attempted"`
	Accepted  int `json:"accepted"`
	Rejected  int `json:"rejected"`
}

type censusRoundTrips struct {
	Attempted         int `json:"attempted"`
	Reparsed          int `json:"reparsed"`
	ParseRejected     int `json:"parse_rejected"`
	SemanticPreserved int `json:"semantic_preserved"`
	SemanticChanged   int `json:"semantic_changed"`
	Stable            int `json:"stable"`
	Unstable          int `json:"unstable"`
}

type databaseCensus struct {
	DeclaredPrograms int              `json:"declared_programs"`
	Unprogrammed     int              `json:"unprogrammed"`
	EmptyPrograms    []censusIdentity `json:"empty_programs"`
	Source           censusCounts     `json:"source"`
	RoundTrips       censusRoundTrips `json:"round_trips"`
	Failures         []censusFailure  `json:"failures"`
	Families         []censusFamily   `json:"families"`
}

// collectDatabaseCensus observes Barn's parser/formatter, not oracle acceptance.
// Keep every failure; only the family summaries limit representative identities.
func collectDatabaseCensus(snapshot store.Snapshot, declared int) databaseCensus {
	report := databaseCensus{DeclaredPrograms: declared, Failures: []censusFailure{}, Families: []censusFamily{}}
	objects := make([]*store.SnapshotObject, 0, len(snapshot.Objects)+len(snapshot.AnonymousObjects))
	for _, object := range snapshot.Objects {
		objects = append(objects, object)
	}
	objects = append(objects, snapshot.AnonymousObjects...)
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
	for _, object := range objects {
		for index, view := range object.VerbList {
			if !view.HasProgram {
				report.Unprogrammed++
				continue
			}
			identity := censusIdentity{object.ID, index, view.Name}
			if len(view.Code) == 0 {
				report.EmptyPrograms = append(report.EmptyPrograms, identity)
			}
			report.Source.Attempted++
			original, err := parser.NewParser(strings.Join(view.Code, "\n")).ParseProgram()
			if err != nil {
				report.Source.Rejected++
				report.reject(identity, "source_parse", err)
				continue
			}
			report.Source.Accepted++
			report.RoundTrips.Attempted++
			formatted := strings.Join(parser.FormatMOO(original), "\n")
			reparsed, err := parser.NewParser(formatted).ParseProgram()
			if err != nil {
				report.RoundTrips.ParseRejected++
				report.reject(identity, "formatted_parse", err)
				continue
			}
			report.RoundTrips.Reparsed++
			if reflect.DeepEqual(withoutPositions(reflect.ValueOf(original)).Interface(), withoutPositions(reflect.ValueOf(reparsed)).Interface()) {
				report.RoundTrips.SemanticPreserved++
			} else {
				report.RoundTrips.SemanticChanged++
				report.reject(identity, "semantic_ir", errors.New("canonical format-parse changed position-free IR"))
			}
			if strings.Join(parser.FormatMOO(reparsed), "\n") == formatted {
				report.RoundTrips.Stable++
			} else {
				report.RoundTrips.Unstable++
				report.reject(identity, "idempotence", errors.New("canonical formatting changed on the second pass"))
			}
		}
	}
	report.groupFailures()
	return report
}

func (report *databaseCensus) reject(identity censusIdentity, stage string, err error) {
	failure := censusFailure{censusIdentity: identity, Stage: stage, Detail: err.Error()}
	var parseError *parser.ParseError
	if errors.As(err, &parseError) {
		// ParseError exposes a line only; do not invent a column or offset.
		failure.Line = parseError.Line
		if parseError.Detail != nil {
			failure.Detail = parseError.Detail.Error()
		}
	}
	failure.Detail = strings.Join(strings.Fields(failure.Detail), " ")
	report.Failures = append(report.Failures, failure)
}

func (report *databaseCensus) groupFailures() {
	groups := make(map[string]*censusFamily)
	for _, failure := range report.Failures {
		key := failure.Stage + "\x00" + failure.Detail
		family := groups[key]
		if family == nil {
			family = &censusFamily{Stage: failure.Stage, Detail: failure.Detail}
			groups[key] = family
		}
		family.Count++
		if len(family.Examples) < 3 {
			family.Examples = append(family.Examples, failure.censusIdentity)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report.Families = append(report.Families, *groups[key])
	}
}

func assertCensusAccounting(t *testing.T, report databaseCensus) {
	t.Helper()
	if report.Source.Attempted != report.DeclaredPrograms || report.Source.Accepted+report.Source.Rejected != report.Source.Attempted {
		t.Errorf("source accounting: declared=%d source=%+v", report.DeclaredPrograms, report.Source)
	}
	round := report.RoundTrips
	if round.Attempted != report.Source.Accepted || round.Reparsed+round.ParseRejected != round.Attempted ||
		round.SemanticPreserved+round.SemanticChanged != round.Reparsed || round.Stable+round.Unstable != round.Reparsed {
		t.Errorf("roundtrip accounting: source=%+v roundtrip=%+v", report.Source, round)
	}
	wantFailures := report.Source.Rejected + round.ParseRejected + round.SemanticChanged + round.Unstable
	familyFailures := 0
	for _, family := range report.Families {
		familyFailures += family.Count
	}
	if len(report.Failures) != wantFailures || familyFailures != wantFailures {
		t.Errorf("failure accounting: records=%d families=%d, want %d", len(report.Failures), familyFailures, wantFailures)
	}
}

func TestDatabaseProgramCensus(t *testing.T) {
	database, err := dbformat.LoadDatabase("../db/format/testdata/toastcore.db")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	report := collectDatabaseCensus(loaded.Snapshot(), database.DeclaredPrograms)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("CENSUS_REPORT\n%s", encoded)
	assertCensusAccounting(t, report)
	if report.DeclaredPrograms != 1950 {
		t.Errorf("tracked fixture declares %d programs, want 1950", report.DeclaredPrograms)
	}
	// The complete inventory makes existing gaps explicit. Changes to acceptance,
	// identities, or formatting require reviewing and updating this artifact.
	want, err := os.ReadFile("testdata/toastcore-census.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected databaseCensus
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report, expected) {
		t.Fatal("database census changed; review the complete report above against testdata/toastcore-census.json")
	}
}

func TestDatabaseCensusIncludesEmptyProgramsAndContinuesPastFiftyAndRejections(t *testing.T) {
	views := make([]store.VerbView, 60)
	for i := range views {
		views[i] = store.VerbView{Name: fmt.Sprintf("valid_%d", i), HasProgram: true, Code: []string{"return 1;"}}
	}
	views = append(views,
		store.VerbView{Name: "empty", HasProgram: true},
		store.VerbView{Name: "never_programmed"},
		store.VerbView{Name: "bad", HasProgram: true, Code: []string{"return 1"}},
		store.VerbView{Name: "after_bad", HasProgram: true, Code: []string{"return 2;"}},
	)
	snapshot := store.Snapshot{Objects: map[types.ObjID]*store.SnapshotObject{
		2: {ID: 2, VerbList: views},
		1: {ID: 1, VerbList: []store.VerbView{{Name: "early_bad", HasProgram: true, Code: []string{"x = 1;", "return 1"}}}},
	}, AnonymousObjects: []*store.SnapshotObject{
		{ID: 9, Anonymous: true, VerbList: []store.VerbView{{Name: "anonymous", HasProgram: true, Code: []string{"return 3;"}}}},
	}}
	report := collectDatabaseCensus(snapshot, 65)
	assertCensusAccounting(t, report)
	if report.Source != (censusCounts{Attempted: 65, Accepted: 63, Rejected: 2}) || report.Unprogrammed != 1 {
		t.Fatalf("incomplete source census: %+v", report)
	}
	if !reflect.DeepEqual(report.EmptyPrograms, []censusIdentity{{2, 60, "empty"}}) {
		t.Fatalf("empty program identities=%+v", report.EmptyPrograms)
	}
	if report.RoundTrips.SemanticPreserved != 63 || report.RoundTrips.Stable != 63 {
		t.Fatalf("incomplete roundtrip census: %+v", report.RoundTrips)
	}
	if len(report.Failures) != 2 || report.Failures[0].ObjectID != 1 || report.Failures[0].VerbIndex != 0 ||
		report.Failures[1].ObjectID != 2 || report.Failures[1].VerbIndex != 62 {
		t.Fatalf("unordered or incomplete rejection identities: %+v", report.Failures)
	}
	if again := collectDatabaseCensus(snapshot, 65); !reflect.DeepEqual(report, again) {
		t.Fatal("census output depends on object map iteration")
	}
}

func TestDatabaseCensusKeepsAllFailuresAndBoundsFamilyExamples(t *testing.T) {
	views := make([]store.VerbView, 10)
	for i := range views {
		views[i] = store.VerbView{Name: fmt.Sprintf("bad_%d", i), HasProgram: true, Code: []string{"return 1"}}
	}
	report := collectDatabaseCensus(store.Snapshot{Objects: map[types.ObjID]*store.SnapshotObject{
		4: {ID: 4, VerbList: views},
	}}, 10)
	assertCensusAccounting(t, report)
	if len(report.Failures) != 10 || len(report.Families) != 1 || report.Families[0].Count != 10 || len(report.Families[0].Examples) != 3 {
		t.Fatalf("failure grouping lost records or exceeded representative bound: %+v", report)
	}
	for index, example := range report.Families[0].Examples {
		if example.ObjectID != 4 || example.VerbIndex != index {
			t.Fatalf("family examples are not the first ordered identities: %+v", report.Families)
		}
	}
}

func TestDatabaseCensusRetainsParseErrorLineAndNormalizesDetail(t *testing.T) {
	identity := censusIdentity{7, 3, "diagnostic"}
	var report databaseCensus
	report.reject(identity, "source_parse", fmt.Errorf("wrapped: %w", &parser.ParseError{
		Line: 17, Msg: "syntax error", Detail: errors.New("  expected\n ';'\t after return statement  "),
	}))
	want := censusFailure{censusIdentity: identity, Stage: "source_parse", Line: 17, Detail: "expected ';' after return statement"}
	if !reflect.DeepEqual(report.Failures, []censusFailure{want}) {
		t.Fatalf("diagnostic lost source position or normalized detail: %+v", report.Failures)
	}
}

func TestDatabaseCensusSortsFamiliesByStageAndDetail(t *testing.T) {
	var report databaseCensus
	identity := censusIdentity{5, 0, "family"}
	report.reject(identity, "source_parse", errors.New("z detail"))
	report.reject(identity, "formatted_parse", errors.New("parse detail"))
	report.reject(identity, "source_parse", errors.New("a detail"))
	report.groupFailures()
	want := []censusFamily{
		{Stage: "formatted_parse", Detail: "parse detail", Count: 1, Examples: []censusIdentity{identity}},
		{Stage: "source_parse", Detail: "a detail", Count: 1, Examples: []censusIdentity{identity}},
		{Stage: "source_parse", Detail: "z detail", Count: 1, Examples: []censusIdentity{identity}},
	}
	if !reflect.DeepEqual(report.Families, want) {
		t.Fatalf("family ordering changed: %+v", report.Families)
	}
}
