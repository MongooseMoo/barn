package builtins

type processUsage struct {
	loadAverage                                         []float64
	userSeconds, systemSeconds                          float64
	minorFaults, majorFaults, inputBlocks, outputBlocks int64
	voluntarySwitches, involuntarySwitches, signals     int64
}

type processMemory struct {
	total, resident, shared, text, data int64
}
