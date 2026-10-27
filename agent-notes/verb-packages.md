# Verb package move

## Goal

Move IR and validation from verb to ir, prepositions to preposition, and migrate
consumers directly, removing command's redundant type/constants. Shorten the
preposition API, delete the unused command wrapper, and relocate stored-verb
and IR-validation declarations within their existing packages. Preserve behavior.
Q authorized end-to-end delivery; parent owns review, PR CI, normal merge and
cleanup. The implementor commits locally and never publishes.

## State

Publication base: origin/master at b91e36f5fe98aa665da9e4ae3ee2bb1f4e847c18;
branch refactor/verb-packages. The original local base
2b6533080740ad0e1301db0008daf23f45ad86e9 included unrelated unpublished research
commits. Parent corrected the delivery prompt to require the remote base.
`git fetch origin master` followed by
`git rebase --onto origin/master 2b653308 refactor/verb-packages` replayed only
the two scoped commits without conflicts. Main checkout/master and unrelated
research commits were preserved. Publication/review must use the rebased head.
The move is complete: four IR/validation files are in ir, two preposition files
are in preposition, and consumers use those owners directly. Command's PrepSpec
and Prep* aliases are removed. Active package documentation is updated.
Initial package move commit: 132fc330bd45658b1852221fa90ae9edbb020899
(formerly 8349d1426a83d4057ed8dcd7db14181960ab2131 before rebase).
The follow-up short API and declaration moves are complete locally. This note
travels with the cleanup commit. No publication has been performed by implementor.
No tests, builds, or CI steps have run for Barn; only semantic tool operations,
source/diff inspection, and package metadata inspection were performed.

## Decisions and why

Q requires proper Go refactoring tooling. The parent's original instruction to
avoid tool writes and apply preview patches was wrong and has been withdrawn.
Do not apply patches or replacement scripts to Barn refactor source. Repair the
actual tooling failure, then use semantic tool write operations. Notes and actual
tool-source repairs may use apply_patch.

The initial Barn change was the approved relocation of roughly 1,005
production lines plus existing tests and direct consumer updates, with no
algorithm changes. Direct comparison of all six moved files against their
original contents found only package declaration changes and the approved IR
package comment change. Existing tests moved unchanged otherwise in that commit.
The approved follow-up adds only symbol/reference edits, the unused wrapper
deletion, and roughly 377 existing lines moving within existing packages.

Semantic commands (gopls and rf previews preceded writes):

- From the worktree: `gopls rename -d verb/ir.go:2:9 ir`, then `-w`.
- From ir: `rf -diff 'mv preposition.go preposition_test.go ../preposition'`,
  then the same script without -diff.
- From command: `rf 'inline -rm PrepSpec PrepWith PrepAt PrepInFrontOf PrepIn PrepOn PrepFrom PrepOver PrepThrough PrepUnder PrepBehind PrepBeside PrepFor PrepIs PrepAs PrepOff PrepNone PrepAny'`.
- rf's supported sub operation updated the IR package comment and the comment
  referring to deleted command aliases. gopls format -w formatted changed Go files.
- `go list -f '{{.ImportPath}} {{.Name}} {{join .Imports ","}}' ./ir ./preposition ./parser ./compiler ./command ./db/format ./builtins`.

Metadata output: `github.com/MongooseMoo/barn/ir ir errors,fmt,reflect` and
`github.com/MongooseMoo/barn/preposition preposition strings`. Parser/compiler
import ir; command, db/format and builtins import preposition. Source search found
no remaining old Barn verb-package imports. These are not CI or runtime results.

## Surprises

gopls v0.17.1 rename -d verb/ir.go:2:9 ir emitted semantic edits then failed:
`gopls: getFile: file:///C:/Users/Q/code/barn-verb-packages/verb: read C:\Users\Q\code\barn-verb-packages\verb: Incorrect function.`
Official v0.17.1 server/rename.go emits a directory RenameFile; cmd/cmd.go handles
it by openFile -> os.ReadFile, causing the error. Official v0.24.0 has the same
CLI handling. The original installed gopls was built with go1.23.4.
The initial Ward delegation had no token; parent confirmed native-tool operation.

The interrupted manual patch initially landed 48 consumer-only edits. All 48
were compared exactly against the captured semantic diff and restored only on
those owned paths through Git before tool-driven writes. No final Go refactor
source edits were applied manually.

## Next step

Parent reviews the combined branch against its base, publishes the PR, reads PR
CI, and coordinates the authorized normal merge and cleanup. Implementation
includes no source-preparation/compiler redesign or runtime algorithm changes.

Tooling repair is local in C:/Users/Q/code/gopls-package-rename on
fix/cli-directory-rename, commit 833c68c75be90ef07d50c54ccd5345f8e666d8b6.
It fixes directory operations, destination parent creation, mapper state after
text edits, and preservation of original backups across file moves. Default
preview/write on Barn returned exit 0. Disposable preserve/write reproduction
returned `rename_exit=0`, `value_backup_exact=True`, `main_backup_exact=True`.
Corrected global install at C:/Users/Q/go/bin/gopls.exe returned `install_exit=0`:
`golang.org/x/tools/gopls v0.24.1-0.20261027004652-833c68c75be9+dirty`.
The +dirty build marker includes untracked candidate/repro artifacts in the tool
checkout; the tool source repair is committed. Original version was v0.17.1.
A prior candidate was prematurely installed before preserve repair, then replaced
with this corrected build. No full tooling test suite was run.

Builds used the official tag's same-checkout developer go.work, not the absent
pinned tools pseudo-version. Initial auto-selection of Go 1.27.0 encountered a
socket access denial; offline resolution lacked dependencies. Parent independently
verified the existing explicit outbound allow rule for cached Go 1.27.1. Normal
build/install with that executable and GOTOOLCHAIN=local then succeeded. No
security policy changes or alternate download routes were used; original denied
process identity is unproven.

The initial rf was built unchanged from upstream commit
85dbf8d53aee26328b0c0d0622d7a6e06f3e4af4 at C:/Users/Q/code/rf-verb-packages/rf.exe.
Its preview needed the existing Git usr/bin directory on command-local PATH for
diff.exe. gopls alone lacks the needed CLI new-package move and alias inlining;
rf supplies those semantic operations directly.

## Mechanical cleanup commands and evidence

All following rf commands were run with `-diff` first, then without it to write.
The preview PATH included `C:/Users/Q/scoop/apps/git/current/usr/bin` for diff.exe.
Executable: `C:/Users/Q/code/rf-verb-packages/rf.exe`.

From preposition, one multiline script:

```text
mv Preposition Spec
mv PrepositionAny Any
mv PrepositionNone None
mv PrepositionWith With
mv PrepositionAt At
mv PrepositionInFrontOf InFrontOf
mv PrepositionIn In
mv PrepositionOn On
mv PrepositionFrom From
mv PrepositionOver Over
mv PrepositionThrough Through
mv PrepositionUnder Under
mv PrepositionBehind Behind
mv PrepositionBeside Beside
mv PrepositionFor For
mv PrepositionIs Is
mv PrepositionAs As
mv PrepositionOff Off
mv Prepositions All
mv PrepositionFromCode FromCode
mv ParsePrepositionAlias ParseAlias
mv ParsePreposition Parse
```

From command: `rf 'rm PrepSpecForAlias'`. Before deletion, repository-wide Go
search returned only `command/command.go:11:func PrepSpecForAlias(...)`.

From db/store:
`rf 'mv Verb VerbView verbIndex buildVerbIndex Object.rebuildVerbIndex verbIndex.lookup scanVerbList Object.findVerbByAlias nextVerbID Object.verbByID NewVerb loweredNames Verb.setCodeOwned Verb.setCodeCopy Verb.View VerbPerms VerbRead VerbWrite VerbExecute VerbDebug VerbPerms.Has VerbPerms.String VerbArgs verb.go'`.

From ir:
`rf 'mv MaxNestingDepth ErrMaxNestingDepth ValidateNesting ValidateNode validateGraph validate.go'`.
The source diff shows exactly the validation block removed from ir.go and added
to validate.go; errors and reflect were already imported by validate.go.
The new db/store/verb.go was read directly after creation.

The store preview first failed with
`panic: object.go:502:1: overlapping edits: [384627,384872)->"", [384627,384872)->""`.
rf's move loop allocated a done map but did not populate it, so constants sharing
one declaration scheduled duplicate edits. The actual tool was repaired by
marking every declObjs member done after moveCode. Local tool commit:
`723035f1db0cdc37c951bc2a77b7f04d136a678e` in C:/Users/Q/code/rf-verb-packages.
First rebuild used Go 1.26.0 (shown by `go version -m rf.exe`) and then failed with
`export data version 4 is greater than maximum supported version 2`.
Rebuilt successfully with `GOTOOLCHAIN=local` and
`C:/Users/Q/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.windows-amd64/bin/go.exe build -o rf.exe .`.
The same store preview and write then each returned exit 0. Preposition rename
and wrapper deletion preview/write also returned exit 0. IR preview returned 0;
its write shell ended at 1 because a subsequent documentation rg found no match,
not because of the move; the resulting diff confirms the write.
No Barn tests, builds, or CI steps were run locally. No performance or isolation
claim is made. Existing preposition test diagnostic strings retain their historic
wording; semantic references were renamed. Active docs needed no further change.
