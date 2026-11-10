package store

import (
	"cmp"
	"slices"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// The census tables tracking adds beside the per-key rows: loss paths, root
// verb goodput and the calls of boundary builtins by site. Like the rest of
// store_conflict_diag.go they are recorded only by transactions begun with
// tracking on, under census.mu, and are bounded: past each table's limit
// further rows count into one overflow row, so the sums stay exact.
//
//   - Loss paths: every stale key of a loss counts into the path (key, loser
//     root verb by its defining object, read site), with the attempt number,
//     where the loss was (final commit, suspend commit, boundary or coarse
//     commit; for a boundary the builtin and its calling site), whether the
//     read was a keyed map read made whole and by what, whether a hot-read
//     renewal had re-read the slot, the writer and how it published, and the
//     wall time from the read to the loss split into exclusive gate wait,
//     shared gate wait and the rest.
//   - Verb goodput: per root verb, slices, attempts and committed/wasted ticks
//     (NoteSliceAttempts), the first boundary of each attempt with its slice
//     ticks and site (NoteBoundary), and retries per finished task
//     (NoteTaskRetries).
//   - Builtin calls: each call of a builtin that crosses the irreversible-effect
//     boundary check, counted by builtin and calling site (NoteBuiltinCall).

const (
	// maxPathBoundarySites bounds the boundary sites counted per path and per
	// verb.
	maxPathBoundarySites = 4
	// maxBuiltinCallSites bounds the (builtin, site) rows of BuiltinCalls.
	maxBuiltinCallSites = 1024
)

// WallBoundsNS are the upper bounds, in nanoseconds, of a WallHist's buckets;
// its last bucket is unbounded.
var WallBoundsNS = [...]int64{1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10}

// WallHist counts wall durations into buckets bounded by WallBoundsNS.
type WallHist [len(WallBoundsNS) + 1]uint64

// Add counts one duration of ns nanoseconds.
func (h *WallHist) Add(ns int64) {
	for i, bound := range WallBoundsNS {
		if ns <= bound {
			h[i]++
			return
		}
	}
	h[len(WallBoundsNS)]++
}

// Quantile returns the upper bound of the bucket holding quantile q (0..1), or
// -1 when that is the unbounded bucket, or 0 when nothing was counted.
func (h WallHist) Quantile(q float64) int64 {
	var total uint64
	for _, n := range h {
		total += n
	}
	if total == 0 {
		return 0
	}
	want := uint64(q*float64(total) + 0.999999)
	var seen uint64
	for i, n := range h {
		seen += n
		if seen >= want {
			if i < len(WallBoundsNS) {
				return WallBoundsNS[i]
			}
			return -1
		}
	}
	return -1
}

// BoundarySiteCount counts irreversible-effect boundaries opened by one builtin
// called at one site, with the slice ticks at each summed.
type BoundarySiteCount struct {
	Builtin string
	Site    ConflictSite
	N       uint64
	Ticks   int64
}

func addBoundarySite(rows *[]BoundarySiteCount, other *uint64, b boundarySite) {
	for i := range *rows {
		if r := &(*rows)[i]; r.Builtin == b.builtin && r.Site == b.site {
			r.N++
			r.Ticks += b.ticks
			return
		}
	}
	if len(*rows) >= maxPathBoundarySites {
		*other++
		return
	}
	*rows = append(*rows, BoundarySiteCount{Builtin: b.builtin, Site: b.site, N: 1, Ticks: b.ticks})
}

func sortBoundarySites(rows []BoundarySiteCount) []BoundarySiteCount {
	out := slices.Clone(rows)
	slices.SortFunc(out, func(a, b BoundarySiteCount) int {
		return cmp.Or(cmp.Compare(b.N, a.N), cmp.Compare(a.Builtin, b.Builtin), compareSite(a.Site, b.Site))
	})
	return out
}

// conflictPathKey is a loss path: the key, the loser's root verb (by the
// object defining it, so every player's `say` is one path) and where the loser
// first read the key.
type conflictPathKey struct {
	conflictKey
	loser ConflictLabel
	site  ConflictSite
}

// ConflictPath is the losses along one path: on one key (Kind, ObjID, Name), by
// tasks running one root verb (Loser: the object defining it and its name),
// that first read the key at ReadSite. ReadSite.Obj is ObjNothing when the read
// site is unknown, which it always is for kinds other than property and
// map-key.
//
// WastedTicks sums the slice ticks at each loss; AttemptSum sums the attempt
// number (1 for a slice's first attempt) of each loss and MaxAttempt is the
// highest. LostAt splits the losses by where validation failed, and
// BoundarySites names, for the boundary losses, the builtin that opened the
// boundary and the site calling it (the first maxPathBoundarySites; the rest
// in OtherBoundarySites). Degraded counts the losses whose read was a keyed
// map read made whole, by path; RenewedThenLost those on a slot a hot-read
// renewal had re-read. Writers holds the first maxPathWriters writers of the
// versions the losers went stale on, most losses first; OtherWriters and
// UnknownWriters count the rest as ConflictDiag does.
//
// WallMeasured counts the losses with a known read site, whose wall time from
// the read to the loss is in ReadToLoss (histogram) and ReadToLossNS (sum);
// ExclusiveWaitNS and SharedWaitNS sum the parts of it the loser spent waiting
// for the commit gate exclusively (the runtime's boundary acquire) and shared
// (at commit). The rest is VM execution and every other wait.
type ConflictPath struct {
	Kind               ConflictKind
	ObjID              types.ObjID
	Name               string
	Loser              ConflictLabel
	ReadSite           ConflictSite
	Lost               uint64
	WastedTicks        int64
	AttemptSum         uint64
	MaxAttempt         int
	LostAt             CommitPointCounts
	BoundarySites      []BoundarySiteCount
	OtherBoundarySites uint64
	Degraded           DegradeCounts
	RenewedThenLost    uint64
	Writers            []ConflictWriterCount
	OtherWriters       uint64
	UnknownWriters     uint64
	WallMeasured       uint64
	ReadToLoss         WallHist
	ReadToLossNS       int64
	ExclusiveWaitNS    int64
	SharedWaitNS       int64
}

// MeanAttempt is the mean attempt number of the path's losses, or 0.
func (p ConflictPath) MeanAttempt() float64 {
	if p.Lost == 0 {
		return 0
	}
	return float64(p.AttemptSum) / float64(p.Lost)
}

func (p *ConflictPath) addWriter(w writerEntry) {
	for i := range p.Writers {
		if row := &p.Writers[i]; row.Label == w.label && row.Site == w.site {
			row.Lost++
			row.PublishedBy.add(w.point)
			return
		}
	}
	if len(p.Writers) >= maxPathWriters {
		p.OtherWriters++
		return
	}
	row := ConflictWriterCount{Label: w.label, Site: w.site, Lost: 1}
	row.PublishedBy.add(w.point)
	p.Writers = append(p.Writers, row)
}

// addPathLocked counts the loss of c, read at key, into its path. The caller
// holds census.mu.
func (tx *StoreTxn) addPathLocked(census *conflictCensus, c *ReadConflict, key propertyReadKey, lossTicks int64, writer writerEntry, writerFound bool, read conflictReadSite, readFound bool) {
	d := tx.diag
	site := ConflictSite{Obj: types.ObjNothing}
	if readFound {
		site = read.site
	}
	path := census.pathLocked(conflictPathKey{
		conflictKey: conflictKey{kind: c.Kind, objID: c.ObjID, name: c.Name},
		loser:       ConflictLabel{Obj: d.definer, Verb: tx.conflictLabel.Verb},
		site:        site,
	})
	path.Lost++
	path.WastedTicks += lossTicks
	path.AttemptSum += uint64(d.attempt)
	path.MaxAttempt = max(path.MaxAttempt, d.attempt)
	path.LostAt.add(d.point)
	if d.point == CommitBoundary {
		addBoundarySite(&path.BoundarySites, &path.OtherBoundarySites, d.boundary)
	}
	if via, ok := d.degraded[key]; ok {
		path.Degraded.add(via)
	}
	if _, ok := d.renewed[key]; ok {
		path.RenewedThenLost++
	}
	if writerFound {
		path.addWriter(writer)
	} else {
		path.UnknownWriters++
	}
	if readFound && !read.at.IsZero() {
		wall := int64(time.Since(read.at))
		path.WallMeasured++
		path.ReadToLoss.Add(wall)
		path.ReadToLossNS += wall
		path.ExclusiveWaitNS += d.exclusiveWaitNS - read.exclusiveWaitNS
		path.SharedWaitNS += d.sharedWaitNS - read.sharedWaitNS
	}
}

// pathLocked returns the row the path key counts into: its own, or the
// overflow row once maxConflictPaths paths are counted. The caller holds
// census.mu.
func (census *conflictCensus) pathLocked(key conflictPathKey) *ConflictPath {
	if p := census.paths[key]; p != nil {
		return p
	}
	if len(census.paths) >= maxConflictPaths {
		return &census.pathOverflow
	}
	p := &ConflictPath{Kind: key.kind, ObjID: key.objID, Name: key.name, Loser: key.loser, ReadSite: key.site}
	lazySet(&census.paths, key, p)
	return p
}

func (p ConflictPath) snapshot() ConflictPath {
	out := p
	out.Writers = slices.Clone(p.Writers)
	slices.SortFunc(out.Writers, func(a, b ConflictWriterCount) int {
		return cmp.Or(cmp.Compare(b.Lost, a.Lost), cmp.Compare(a.Label.Obj, b.Label.Obj),
			cmp.Compare(a.Label.Verb, b.Label.Verb), compareSite(a.Site, b.Site))
	})
	out.BoundarySites = sortBoundarySites(p.BoundarySites)
	return out
}

// SetConflictOrigin tells a tracking transaction which object defines the
// task's root verb and which attempt of its slice it is (1 for the first), for
// the loss paths. It does nothing on a transaction begun with tracking off.
func (tx *StoreTxn) SetConflictOrigin(definer types.ObjID, attempt int) {
	if tx != nil && tx.diag != nil {
		tx.diag.definer, tx.diag.attempt = definer, attempt
	}
}

// RetryBucketBounds are the inclusive upper bounds of VerbGoodput.RetriesPerTask's
// buckets (retries of one finished task); its last bucket is unbounded.
var RetryBucketBounds = [...]int{0, 1, 2, 3, 7, 15, 31}

// VerbGoodput is what one root verb's tasks did: Verb names it by the object
// defining it and its name. Slices counts the slices that resolved (committed,
// or failed for good) and Attempts their attempts, so Attempts - Slices is the
// retries. CommittedTicks sums the ticks of each slice's resolving attempt and
// WastedTicks those of the attempts discarded and re-run.
//
// Boundaries counts the attempts that crossed an irreversible-effect boundary
// and BoundaryTicks sums the slice ticks at each first crossing; BoundarySites
// names the builtins and calling sites (the first maxPathBoundarySites, the
// rest in OtherBoundarySites). Tasks counts the finished tasks, RetriesPerTask
// their retries over all their slices in RetryBucketBounds buckets, and
// MaxRetriesPerTask the most.
type VerbGoodput struct {
	Verb               ConflictLabel
	Slices             uint64
	Attempts           uint64
	CommittedTicks     int64
	WastedTicks        int64
	Boundaries         uint64
	BoundaryTicks      int64
	BoundarySites      []BoundarySiteCount
	OtherBoundarySites uint64
	Tasks              uint64
	RetriesPerTask     [len(RetryBucketBounds) + 1]uint64
	MaxRetriesPerTask  int
}

// Retries is Attempts - Slices.
func (v VerbGoodput) Retries() uint64 { return v.Attempts - v.Slices }

// WastedShare is WastedTicks as a percentage of all ticks the verb ran, or 0.
func (v VerbGoodput) WastedShare() float64 {
	total := v.CommittedTicks + v.WastedTicks
	if total <= 0 {
		return 0
	}
	return 100 * float64(v.WastedTicks) / float64(total)
}

func (v *VerbGoodput) add(attempts int, ticks, rerunTicks int64) {
	v.Slices++
	v.Attempts += uint64(attempts)
	v.CommittedTicks += ticks - rerunTicks
	v.WastedTicks += rerunTicks
}

func (v *VerbGoodput) addBoundarySite(b boundarySite) {
	addBoundarySite(&v.BoundarySites, &v.OtherBoundarySites, b)
}

func (v VerbGoodput) snapshot() VerbGoodput {
	out := v
	out.BoundarySites = sortBoundarySites(v.BoundarySites)
	return out
}

// verbRowLocked returns the goodput row of root verb verb, or the overflow row
// once maxGoodputVerbs verbs are counted. The caller holds census.mu.
func (census *conflictCensus) verbRowLocked(verb ConflictLabel) *VerbGoodput {
	if row := census.verbs[verb]; row != nil {
		return row
	}
	if len(census.verbs) >= maxGoodputVerbs {
		return &census.verbOverflow
	}
	row := &VerbGoodput{Verb: verb}
	lazySet(&census.verbs, verb, row)
	return row
}

// NoteTaskRetries counts one finished task of root verb verb that retried
// retries times over all its slices. It does nothing unless tracking is on.
func (s *Store) NoteTaskRetries(verb ConflictLabel, retries int) {
	if !s.conflicts.tracking.Load() || retries < 0 {
		return
	}
	census := &s.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	row := census.verbRowLocked(verb)
	row.Tasks++
	bucket := len(RetryBucketBounds)
	for i, bound := range RetryBucketBounds {
		if retries <= bound {
			bucket = i
			break
		}
	}
	row.RetriesPerTask[bucket]++
	row.MaxRetriesPerTask = max(row.MaxRetriesPerTask, retries)
}

// BuiltinCallCount counts the calls of one builtin from one site.
type BuiltinCallCount struct {
	Builtin string
	Site    ConflictSite
	Calls   uint64
}

type builtinCallKey struct {
	builtin string
	site    ConflictSite
}

// NoteBuiltinCall counts a call of builtin, a builtin that checks the
// irreversible-effect boundary before acting, by the site executing now. It
// does nothing on a transaction begun with tracking off or without a site
// callback.
func (tx *StoreTxn) NoteBuiltinCall(builtin string) {
	if tx == nil || tx.diag == nil || tx.diag.siteFn == nil {
		return
	}
	site, _ := tx.diag.siteFn()
	key := builtinCallKey{builtin: builtin, site: site}
	census := &tx.store.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	if _, ok := census.builtinCalls[key]; !ok && len(census.builtinCalls) >= maxBuiltinCallSites {
		census.builtinCallOverflow++
		return
	}
	if census.builtinCalls == nil {
		census.builtinCalls = make(map[builtinCallKey]uint64)
	}
	census.builtinCalls[key]++
}

// builtinCallsLocked returns the builtin call rows, most calls first. The
// caller holds census.mu.
func (census *conflictCensus) builtinCallsLocked() []BuiltinCallCount {
	if len(census.builtinCalls) == 0 {
		return nil
	}
	out := make([]BuiltinCallCount, 0, len(census.builtinCalls))
	for key, n := range census.builtinCalls {
		out = append(out, BuiltinCallCount{Builtin: key.builtin, Site: key.site, Calls: n})
	}
	slices.SortFunc(out, func(a, b BuiltinCallCount) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), cmp.Compare(a.Builtin, b.Builtin), compareSite(a.Site, b.Site))
	})
	return out
}
