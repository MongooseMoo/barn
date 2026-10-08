package vm

import (
	"fmt"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestRecycleFrozenAnonymousCandidatesPreservesRouting(t *testing.T) {
	context := func(programmer types.ObjID) *kernel.TaskContext {
		ctx := kernel.NewTaskContext()
		ctx.Programmer = programmer
		return ctx
	}
	a, b, c := context(10), context(20), context(30)
	for _, test := range []struct {
		name       string
		requests   []AnonGCRequest
		candidates []types.ObjID
		want       []string
	}{
		{"empty", []AnonGCRequest{{Ctx: a}}, nil, nil},
		{"no context", []AnonGCRequest{{MinID: 0}}, []types.ObjID{1}, nil},
		{"uncovered", []AnonGCRequest{{Ctx: a, MinID: 7}}, []types.ObjID{1, 3, 5}, nil},
		{"first eligible context and frozen order", []AnonGCRequest{
			{MinID: 0}, {Ctx: a, MinID: 7}, {Ctx: b, MinID: 3},
			{Ctx: c, MinID: 1}, {Ctx: a, MinID: 0},
		}, []types.ObjID{5, 1, 9, 3, 7}, []string{"10:9", "10:7", "20:5", "20:3", "30:1"}},
		{"remaining candidates need later floor", []AnonGCRequest{
			{Ctx: a, MinID: 7}, {Ctx: b, MinID: 8}, {Ctx: c, MinID: 1},
		}, []types.ObjID{5, 1, 9, 3, 7}, []string{"10:9", "10:7", "30:5", "30:1", "30:3"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			recycleFrozenAnonymousCandidates(test.requests, test.candidates, func(req AnonGCRequest, id types.ObjID) {
				got = append(got, fmt.Sprintf("%d:%d", req.Ctx.Programmer, id))
			})
			if !slices.Equal(got, test.want) {
				t.Fatalf("recycle routes = %v, want %v", got, test.want)
			}
		})
	}
}

// A fixed frozen list and acknowledged callback digest isolate request-count
// scaling from store mutation, hooks, scheduler timing, and database tracing.
func BenchmarkRecycleFrozenAnonymousCandidates(b *testing.B) {
	const candidateCount = 1024
	candidates := make([]types.ObjID, candidateCount)
	var wantSum uint64
	for i := range candidates {
		candidates[i] = types.ObjID(i + 1)
		wantSum += uint64(i + 1)
	}
	ctx := kernel.NewTaskContext()
	for _, requestCount := range []int{1, 128, 4096} {
		b.Run(fmt.Sprintf("requests_%d", requestCount), func(b *testing.B) {
			requests := make([]AnonGCRequest, requestCount)
			for i := range requests {
				requests[i] = AnonGCRequest{Ctx: ctx, MinID: 1}
			}
			var calls, sum uint64
			recycle := func(_ AnonGCRequest, id types.ObjID) { calls++; sum += uint64(id) }
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				recycleFrozenAnonymousCandidates(requests, candidates, recycle)
			}
			b.StopTimer()
			if calls != uint64(b.N)*candidateCount || sum != uint64(b.N)*wantSum {
				b.Fatalf("callback digest = %d/%d, want %d/%d", calls, sum, uint64(b.N)*candidateCount, uint64(b.N)*wantSum)
			}
		})
	}
}
