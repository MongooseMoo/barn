package server

import (
	"log/slog"

	"github.com/MongooseMoo/barn/metrics"
)

func (p *InputProcessor) commandTaskStarted(conn *Connection, diagnostic *metrics.Command) func(int64) {
	return func(id int64) {
		conn.SetLastInputTaskID(id)
		p.runtime.BindCommandDiagnostic(id, diagnostic)
	}
}

// These independently sampled fields describe overlapping work, not an atomic
// snapshot or proof that any listed task caused the input's delay.
func (p *InputProcessor) commandContext() []slog.Attr {
	stats := p.runtime.AdmissionStats()
	tasks := p.runtime.UnfinishedTaskSummaries()
	omitted := max(0, len(tasks)-64)
	if len(tasks) > 64 {
		tasks = tasks[:64]
	}
	return []slog.Attr{slog.Int("admission_active", stats.Active), slog.Int("admission_queued", stats.Queued),
		slog.Int("admission_preempted", stats.Preempted), slog.Any("overlapping_tasks", tasks), slog.Int("overlapping_tasks_omitted", omitted)}
}
