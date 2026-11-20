package compiler_test

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/parser"
)

// The fixture is raw stock-Toast disassemble output from the managed producer
// beside it. Each numeric call opcode's next instruction supplies the exact
// continuation PC, independently of the layout implementation under test.
func TestNativeLayoutMatchesToastDisassembly(t *testing.T) {
	raw, err := os.ReadFile("testdata/native-layout-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries [][3]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		var name string
		var source, rows []string
		if err := json.Unmarshal(entry[0], &name); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(entry[1], &source); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(entry[2], &rows); err != nil {
			t.Fatal(err)
		}
		t.Run(name, func(t *testing.T) {
			program, err := parser.NewParser(strings.Join(source, "\n")).ParseProgram()
			if err != nil {
				t.Fatal(err)
			}
			layout, err := compiler.NativeLayout(program, nil, 17)
			if err != nil {
				t.Fatal(err)
			}
			want := nativeDisassemblyVectors(t, rows)
			if len(layout.Vectors) != len(want) {
				t.Fatalf("vectors=%d want %d", len(layout.Vectors), len(want))
			}
			for _, got := range layout.Vectors {
				expected := want[got.Vector]
				if got.Size != expected.Size || got.MaxStack != expected.MaxStack || got.LiteralWidth != expected.LiteralWidth || got.ForkWidth != expected.ForkWidth || got.VariableWidth != expected.VariableWidth || got.LabelWidth != expected.LabelWidth || got.StackWidth != expected.StackWidth {
					t.Errorf("vector %d metrics={size:%d max:%d literal:%d fork:%d var:%d label:%d stack:%d} want={size:%d max:%d literal:%d fork:%d var:%d label:%d stack:%d}", got.Vector, got.Size, got.MaxStack, got.LiteralWidth, got.ForkWidth, got.VariableWidth, got.LabelWidth, got.StackWidth, expected.Size, expected.MaxStack, expected.LiteralWidth, expected.ForkWidth, expected.VariableWidth, expected.LabelWidth, expected.StackWidth)
				}
				var pcs []int
				for _, site := range got.Continuations {
					pcs = append(pcs, site.PC)
				}
				if !reflect.DeepEqual(pcs, expected.PCs) {
					t.Errorf("vector %d continuation PCs=%v want %v", got.Vector, pcs, expected.PCs)
				}
			}
		})
	}
}

type nativeOracleVector struct {
	compiler.NativeVectorLayout
	PCs []int
}

func nativeDisassemblyVectors(t *testing.T, rows []string) map[int]*nativeOracleVector {
	t.Helper()
	fork := regexp.MustCompile(`^Forked code vector ([0-9]+):$`)
	widths := regexp.MustCompile(`^\[Bytes for labels = ([0-9]+), literals = ([0-9]+), forks = ([0-9]+), variables = ([0-9]+), stack refs = ([0-9]+)\]$`)
	stack := regexp.MustCompile(`^\[Maximum stack size = ([0-9]+)\]$`)
	instruction := regexp.MustCompile(`^\s*([0-9]+):\s+([0-9]{3})(?:\s|$)`)
	result := make(map[int]*nativeOracleVector)
	var current *nativeOracleVector
	previousCall := false
	for _, row := range rows {
		if row == "Main code vector:" {
			current = &nativeOracleVector{}
			result[-1] = current
			previousCall = false
			continue
		}
		if match := fork.FindStringSubmatch(row); match != nil {
			id, _ := strconv.Atoi(match[1])
			current = &nativeOracleVector{}
			result[id] = current
			previousCall = false
			continue
		}
		if current == nil {
			continue
		}
		if match := widths.FindStringSubmatch(row); match != nil {
			values := make([]int, 5)
			for i := range values {
				values[i], _ = strconv.Atoi(match[i+1])
			}
			current.LabelWidth = values[0]
			current.LiteralWidth = values[1]
			current.ForkWidth = values[2]
			current.VariableWidth = values[3]
			current.StackWidth = values[4]
		}
		if match := stack.FindStringSubmatch(row); match != nil {
			current.MaxStack, _ = strconv.Atoi(match[1])
		}
		if match := instruction.FindStringSubmatch(row); match != nil {
			pc, _ := strconv.Atoi(match[1])
			opcode, _ := strconv.Atoi(match[2])
			current.Size = pc + 1
			if previousCall {
				current.PCs = append(current.PCs, pc)
			}
			previousCall = opcode == 10 || opcode == 12
		}
	}
	if len(result) == 0 {
		t.Fatal("native oracle fixture has no code vectors")
	}
	return result
}

func TestNativeLayoutMatchesToastSuspendedProducer(t *testing.T) {
	database, err := dbformat.LoadDatabase("../engine/testdata/toast-suspended-call.db")
	if err != nil {
		t.Fatal(err)
	}
	for frameIndex, saved := range database.SuspendedTasks[0].Snapshot.VM.Frames {
		program, err := parser.NewParser(strings.Join(saved.Program.Source, "\n")).ParseProgram()
		if err != nil {
			t.Fatal(err)
		}
		layout, err := compiler.NativeLayout(program, saved.Program.VarNames, 17)
		if err != nil {
			t.Fatal(err)
		}
		vector := -1
		if frameIndex == 0 {
			vector = 0
		}
		found := false
		for _, candidate := range layout.Vectors {
			if candidate.Vector != vector {
				continue
			}
			for _, site := range candidate.Continuations {
				if site.PC == saved.IP {
					found = true
					if len(site.Stack) != len(saved.Stack)+1 {
						t.Fatalf("frame %d native PC %d logical stack=%d saved=%d", frameIndex, saved.IP, len(site.Stack), len(saved.Stack))
					}
				}
			}
		}
		if !found {
			t.Errorf("frame %d vector %d native PC %d absent from exact layout", frameIndex, vector, saved.IP)
		}
	}
}

func TestNativeLayoutMatchesOriginalCheckpoint(t *testing.T) {
	path := os.Getenv("BARN_MONGOOSE_DB")
	if path == "" {
		t.Skip("set BARN_MONGOOSE_DB for original native continuation census")
	}
	database, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, sites := 0, 0
	for _, savedTask := range database.SuspendedTasks {
		for frameIndex, saved := range savedTask.Snapshot.VM.Frames {
			if saved.Native == nil {
				continue
			}
			frames++
			program, err := parser.NewParser(strings.Join(saved.Program.Source, "\n")).ParseProgram()
			if err != nil {
				t.Errorf("task %d frame%d parse: %v", savedTask.Snapshot.ID, frameIndex, err)
				continue
			}
			layout, err := compiler.NativeLayout(program, saved.Program.VarNames, saved.Native.LanguageVersion)
			if err != nil {
				t.Errorf("task %d frame%d layout: %v", savedTask.Snapshot.ID, frameIndex, err)
				continue
			}
			vector := -1
			if frameIndex == 0 {
				vector = savedTask.Snapshot.VM.NativeRootVector
			}
			found := false
			for _, candidate := range layout.Vectors {
				if candidate.Vector != vector {
					continue
				}
				for _, site := range candidate.Continuations {
					if site.PC != saved.IP {
						continue
					}
					found = true
					sites++
					if len(site.Stack) != len(saved.Stack)+1 {
						t.Errorf("task%d frame%d nativePC%d logicalstack%d saved%d", savedTask.Snapshot.ID, frameIndex, saved.IP, len(site.Stack), len(saved.Stack))
					}
				}
			}
			if !found {
				t.Errorf("task%d frame%d vector%d nativePC%d has no continuation", savedTask.Snapshot.ID, frameIndex, vector, saved.IP)
			}
		}
	}
	t.Logf("native frames=%d matched continuations=%d", frames, sites)
	if frames == 0 || sites == 0 {
		t.Fatalf("opted-in native continuation census requires nonzero frames and matches; got frames=%d matches=%d", frames, sites)
	}
}
