package engine

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func BenchmarkTaskManagerScans(b *testing.B) {
	const player types.ObjID = 7
	for _, completed := range []int{0, 10_000} {
		b.Run(fmt.Sprintf("completed=%d", completed), func(b *testing.B) {
			manager := NewTaskManager()
			for id := int64(1); id <= 10; id++ {
				candidate := NewTask(id, player, 100, 5)
				candidate.QueueSeq = id
				candidate.SetReadingPlayer(player)
				candidate.SetState(task.TaskSuspended)
				manager.RegisterTask(candidate)
			}
			for id := int64(11); id <= int64(completed)+10; id++ {
				candidate := NewTask(id, player, 100, 5)
				candidate.SetState(task.TaskCompleted)
				manager.RegisterTask(candidate)
			}
			b.Run("FindReadingTask", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					manager.FindReadingTask(player)
				}
			})
			b.Run("GetQueuedTasks", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					manager.GetQueuedTasks()
				}
			})
		})
	}
}
