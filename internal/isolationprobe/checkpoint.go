package isolationprobe

// Checkpoint pause points, one per boundary of the Jepsen checkpoint
// interruption table. Each name is bound to one call site in db/format.
const (
	// CheckpointCapture: the world/task snapshot is taken, still inside the
	// checkpoint barrier; nothing is written to the temporary file yet.
	CheckpointCapture = "checkpoint/capture"
	// CheckpointSerialize: the temporary database is written but not synced;
	// no sidecar exists yet.
	CheckpointSerialize = "checkpoint/serialize"
	// CheckpointStaged: the temporary database is synced and its sidecar written
	// and hashed; no recovery generation or journal exists yet.
	CheckpointStaged = "checkpoint/staged"
	// CheckpointJournalPublished: the publication journal is renamed into place
	// and its directory synced; the output database is not yet replaced.
	CheckpointJournalPublished = "checkpoint/journal-published"
	// CheckpointDatabaseRenamed: the output database is replaced; the sidecar
	// rename has not happened.
	CheckpointDatabaseRenamed = "checkpoint/database-renamed"
	// CheckpointPairRenamed: both renames and the directory sync succeeded; the
	// journal and recovery generation are not yet removed.
	CheckpointPairRenamed = "checkpoint/pair-renamed"
	// CheckpointRecoveryDatabaseRestored: recovery restored the saved database
	// over the output; the saved sidecar rename has not happened.
	CheckpointRecoveryDatabaseRestored = "checkpoint/recovery-database-restored"
)

// CheckpointPoints lists every checkpoint pause point.
var CheckpointPoints = []string{
	CheckpointCapture,
	CheckpointSerialize,
	CheckpointStaged,
	CheckpointJournalPublished,
	CheckpointDatabaseRenamed,
	CheckpointPairRenamed,
	CheckpointRecoveryDatabaseRestored,
}
