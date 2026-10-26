package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func BenchmarkBuiltinAdmission(b *testing.B) {
	for _, name := range []string{"length", "strsub"} {
		b.Run(name, func(b *testing.B) {
			s := NewSession(NewRegistry(), NoHost())
			defer s.Close()
			ctx := s.NewExecution(&task.Context{}, nil)
			id, _ := s.Registry().GetID(name)
			args := []types.Value{types.NewStr("abc")}
			if name == "strsub" {
				args = append(args, types.NewStr("b"), types.NewStr("d"))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s.CallByIDWithExecution(id, ctx, args)
			}
		})
	}
}
