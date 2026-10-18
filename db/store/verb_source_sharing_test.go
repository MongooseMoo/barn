package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Constructor and loader source may still have an external alias. A read clone
// must establish a stable source/key pair before any later clones can retain it.
func TestVerbSourceClonesDetachBorrowedInputs(t *testing.T) {
	for _, loader := range []bool{false, true} {
		name := "constructor"
		if loader {
			name = "loader"
		}
		t.Run(name, func(t *testing.T) {
			names := []string{"look", "glance"}
			code := []string{"x = 7;", "return x;"}
			v := NewVerb("look", names, 0, VerbRead|VerbExecute, publicationArgsA, code)
			if loader {
				v.SetCode(code)
			}
			first := cloneVerbForReadTxn(&v)
			second := cloneVerbForReadTxn(first)
			want := ownPublicationView(first.View())
			names[0], code[0] = "external-name", "return 999;"
			requirePublicationView(t, first.View(), want)
			requirePublicationView(t, second.View(), want)
			// A clone is a private mutable node, including its name collection.
			first.names[0] = "private-name"
			requirePublicationView(t, second.View(), want)
		})
	}
}

func TestVerbSourceSequentialClonesPreserveRetainedSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		install func(*Verb, []string)
	}{
		{"SetCode", (*Verb).SetCode},
		{"owned", (*Verb).setCodeOwned},
		{"copy", (*Verb).setCodeCopy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := NewVerb("look", []string{"look", "glance"}, 0, VerbRead|VerbExecute,
				publicationArgsA, []string{"return 1;"})
			first := cloneVerbForReadTxn(&v)
			second := cloneVerbForReadTxn(first)
			oldFirst, oldSecond := first.View(), second.View()
			wantOld := ownPublicationView(oldFirst)
			input := []string{"return 73;"}
			tc.install(first, input)
			third := cloneVerbForReadTxn(first)
			wantNew := ownPublicationView(third.View())
			if wantNew.Code[0] != "return 73;" || wantNew.CodeKey == wantOld.CodeKey || !wantNew.HasProgram {
				t.Fatal("replacement source did not install its own program identity")
			}
			// Even an ownership-transfer caller can retain an input reference. The
			// next read clone must detach that replacement before publishing it.
			input[0] = "return 999;"
			requirePublicationView(t, third.View(), wantNew)
			requirePublicationView(t, oldFirst, wantOld)
			requirePublicationView(t, oldSecond, wantOld)
			second.setCodeCopy([]string{"return 42;"})
			requirePublicationView(t, oldSecond, wantOld)
			requirePublicationView(t, third.View(), wantNew)
		})
	}
}

// Exercise real adoption and code publication, retaining the read-only compiler
// input slices from before each replacement. No source-sharing identity is part
// of this contract: unchanged and optimized clone implementations must pass.
func TestVerbSourceAdoptionAndLaterCodeReplacement(t *testing.T) {
	for _, mode := range []string{"direct-name", "direct-index", "snapshot", "coarse"} {
		t.Run(mode, func(t *testing.T) {
			s := newVerbArgsPublicationStore(t, 2, 2)
			if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
				t.Fatal(ec)
			}
			reader := s.BeginSnapshot(0)
			defer reader.Release()
			old, _, err := reader.FindCallableVerb(0, "v000")
			if err != nil {
				t.Fatal(err)
			}
			wantOld := ownPublicationView(old)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx.MarkLiveMutated()
			if ec := tx.AdoptLiveVerbs(0); ec != types.E_NONE {
				t.Fatal(ec)
			}
			adopted, _, err := tx.FindCallableVerb(0, "alias_v000")
			if err != nil || adopted.ArgSpec != publicationArgsB {
				t.Fatalf("adopted argument metadata: %v, %v", adopted.ArgSpec, err)
			}
			wantAdopted := ownPublicationView(adopted)
			stagedInput := []string{"return 73;"}
			if ec := tx.SetVerbCode(0, "v000", stagedInput); ec != types.E_NONE {
				t.Fatal(ec)
			}
			stagedInput[0] = "return 999;"
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			committed := publicationVerb(t, s, 0)
			wantCommitted := ownPublicationView(committed)
			if len(committed.Code) != 1 || committed.Code[0] != "return 73;" || committed.CodeKey == old.CodeKey {
				t.Fatal("staged replacement source/key was not published intact")
			}
			next := []string{"return 42;"}
			var ec types.ErrorCode
			switch mode {
			case "direct-name":
				ec = s.DirectTxn().SetVerbCode(0, "v000", next)
			case "direct-index":
				ec = s.DirectTxn().SetVerbCodeByIndex(0, 0, next)
			default:
				writer := s.BeginSnapshot(0)
				defer writer.Release()
				if ec = writer.SetVerbCode(0, "v000", next); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if mode == "coarse" {
					writer.MarkLiveMutated()
				}
				ec = writer.Commit()
			}
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			next[0] = "return 1000;"
			latest := publicationVerb(t, s, 0)
			if len(latest.Code) != 1 || latest.Code[0] != "return 42;" || latest.CodeKey == committed.CodeKey {
				t.Fatal("later replacement source/key was not published intact")
			}
			assertKeyMatchesCode(t, latest, mode)
			requirePublicationView(t, old, wantOld)
			requirePublicationView(t, adopted, wantAdopted)
			requirePublicationView(t, committed, wantCommitted)
			again, _, err := reader.FindCallableVerb(0, "v000")
			if err != nil {
				t.Fatal(err)
			}
			requirePublicationView(t, again, wantOld)
		})
	}
}
