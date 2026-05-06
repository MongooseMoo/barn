package db

import (
	"barn/types"
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// readFinalizations reads pending finalizations (v17)
func (db *Database) readFinalizations(r *bufio.Reader) error {
	// Format: "N values pending finalization"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	var count int
	if _, err := fmt.Sscanf(line, "%d values pending finalization", &count); err != nil {
		return fmt.Errorf("parse pending finalization count: %w", err)
	}

	db.PendingFinalizations = make([]types.Value, 0, count)
	for i := 0; i < count; i++ {
		val, err := db.readValue(r)
		if err != nil {
			return fmt.Errorf("read pending finalization %d: %w", i, err)
		}
		if objVal, ok := val.(types.ObjValue); ok && objVal.IsAnonymous() && objVal.ID() < 0 {
			continue
		}
		db.PendingFinalizations = append(db.PendingFinalizations, val)
	}
	return nil
}

// readClocks reads clocks section (obsolete)
func (db *Database) readClocks(r *bufio.Reader) error {
	// Format: "N clocks" where N is usually 0
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	// We just skip this line - clocks are obsolete
	_ = line
	return nil
}

// readQueuedTasks reads queued tasks
func (db *Database) readQueuedTasks(r *bufio.Reader) error {
	// Format: "N queued tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N queued tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d queued tasks", &count)
	if err != nil {
		return fmt.Errorf("parse queued tasks count: %w", err)
	}

	db.QueuedTasks = make([]*QueuedTask, 0, count)
	for i := 0; i < count; i++ {
		// Skip task data for now - just read until terminator
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
		}
	}
	return nil
}

// readSuspendedTasks reads suspended tasks
func (db *Database) readSuspendedTasks(r *bufio.Reader) error {
	// Format: "N suspended tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N suspended tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d suspended tasks", &count)
	if err != nil {
		return fmt.Errorf("parse suspended tasks count: %w", err)
	}

	db.SuspendedTasks = make([]*SuspendedTask, 0, count)
	for i := 0; i < count; i++ {
		if err := db.skipSuspendedTask(r); err != nil {
			return fmt.Errorf("skip suspended task %d: %w", i, err)
		}
	}
	return nil
}

// skipSuspendedTask skips over a complete suspended task in the database file.
// A suspended task contains a VM with multiple activations (stack frames),
// each terminated by a period. We must parse the VM header to know how many
// activations to skip.
func (db *Database) skipSuspendedTask(r *bufio.Reader) error {
	// Task header: "<start_time> <task_id> <type_code>"
	// The type_code is the start of the suspend value.
	// Format: "1767134605 2112268937 0" where 0 is INT type code
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read task header: %w", err)
	}

	// Parse the header: time, taskid, and optional type code
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 {
		return fmt.Errorf("parse task header: expected at least 2 fields, got %d from %q", len(parts), line)
	}

	// If there's a third part, it's the type code for suspend value
	if len(parts) >= 3 {
		typeCode, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("parse suspend value type: %w", err)
		}
		// Read the rest of the suspend value (type code already parsed)
		if err := db.skipValueAfterType(r, typeCode); err != nil {
			return fmt.Errorf("read suspend value: %w", err)
		}
	}

	// Read VM local var
	if _, err := db.readValue(r); err != nil {
		return fmt.Errorf("read VM local: %w", err)
	}

	// Read VM header: "<top_activ_stack> <root_activ_vector> <func_id>[ <max_stack_size>]"
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read VM header: %w", err)
	}

	var topActivStack, rootActivVector, funcID int
	n, _ := fmt.Sscanf(line, "%d %d %d", &topActivStack, &rootActivVector, &funcID)
	if n < 3 {
		return fmt.Errorf("parse VM header: got %d fields from %q", n, line)
	}

	// Read activations: indices 0 through topActivStack (inclusive)
	numActivations := topActivStack + 1
	for a := 0; a < numActivations; a++ {
		if err := db.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// skipActivation skips over a single activation (stack frame) in a suspended task.
func (db *Database) skipActivation(r *bufio.Reader) error {
	// "language version N"
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read language version: %w", err)
	}
	if !strings.HasPrefix(line, "language version") {
		return fmt.Errorf("expected 'language version', got %q", line)
	}

	// Read verb code until "."
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read verb code: %w", err)
		}
		if strings.TrimSpace(line) == "." {
			break
		}
	}

	// "N variables"
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read variables header: %w", err)
	}
	var numVars int
	if _, err := fmt.Sscanf(line, "%d variables", &numVars); err != nil {
		return fmt.Errorf("parse variables count from %q: %w", line, err)
	}

	// Read variable definitions: each is a name line followed by a typed value.
	for i := 0; i < numVars; i++ {
		// Variable name
		if _, err = r.ReadString('\n'); err != nil {
			return fmt.Errorf("read variable %d name: %w", i, err)
		}
		// Variable value (type code + value)
		if _, err := db.readValue(r); err != nil {
			return fmt.Errorf("read variable %d value: %w", i, err)
		}
	}

	// "N rt_stack slots in use"
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read rt_stack header: %w", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(line), "rt_stack slots in use") {
		return fmt.Errorf("expected 'rt_stack slots in use', got %q", line)
	}

	// Parse rt_stack count from the line we just read
	var numStackSlots int
	fmt.Sscanf(line, "%d rt_stack slots in use", &numStackSlots)

	// Skip stack slot values
	for i := 0; i < numStackSlots; i++ {
		if _, err := db.readValue(r); err != nil {
			return fmt.Errorf("read stack slot %d: %w", i, err)
		}
	}

	// Skip activation info (activ_as_pi)
	// Format from Toast write_activ_as_pi:
	// 1. dummy value (INT -111)
	// 2. _this value
	// 3. vloc value
	// 4. threaded number
	// 5. verbref line: "recv -7 -8 player -9 progr vloc -10 debug"
	// 6. 4 strings (No, More, Parse, Infos)
	// 7. verb name string
	// 8. verb aliases string

	// Read 3 MOO values (dummy, _this, vloc)
	for i := 0; i < 3; i++ {
		if _, err := db.readValue(r); err != nil {
			return fmt.Errorf("read activ value %d: %w", i, err)
		}
	}

	// Read threaded number
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read threaded: %w", err)
	}

	// Read verbref line
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verbref: %w", err)
	}

	// Read 4 placeholder strings (No, More, Parse, Infos)
	for i := 0; i < 4; i++ {
		if _, err = r.ReadString('\n'); err != nil {
			return fmt.Errorf("read placeholder string %d: %w", i, err)
		}
	}

	// Read verb name and aliases (2 strings)
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verb name: %w", err)
	}
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verb aliases: %w", err)
	}

	// Read temp value
	if _, err := db.readValue(r); err != nil {
		return fmt.Errorf("read temp value: %w", err)
	}

	// Read PC info line
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read PC info: %w", err)
	}

	return nil
}

// readInterruptedTasks reads interrupted tasks
func (db *Database) readInterruptedTasks(r *bufio.Reader) error {
	// Format: "N interrupted tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N interrupted tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d interrupted tasks", &count)
	if err != nil {
		return fmt.Errorf("parse interrupted tasks count: %w", err)
	}

	// Skip interrupted task data
	for i := 0; i < count; i++ {
		if err := db.skipInterruptedTask(r); err != nil {
			return fmt.Errorf("skip interrupted task %d: %w", i, err)
		}
	}
	return nil
}

// skipInterruptedTask skips over a complete interrupted task.
// Format: "<task_id> <status_string>\n" followed by a VM.
func (db *Database) skipInterruptedTask(r *bufio.Reader) error {
	// Task header: "<task_id> <status_string>"
	// e.g., "1638619699 interrupted reading task"
	if _, err := r.ReadString('\n'); err != nil {
		return fmt.Errorf("read task header: %w", err)
	}

	// Read VM (same as suspended task VM, but no suspend value)
	// VM local var
	if _, err := db.readValue(r); err != nil {
		return fmt.Errorf("read VM local: %w", err)
	}

	// VM header
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read VM header: %w", err)
	}

	var topActivStack int
	if n, _ := fmt.Sscanf(line, "%d", &topActivStack); n < 1 {
		return fmt.Errorf("parse VM header from %q", line)
	}

	// Read activations
	numActivations := topActivStack + 1
	for a := 0; a < numActivations; a++ {
		if err := db.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// readActiveConnections reads active connections
func (db *Database) readActiveConnections(r *bufio.Reader) error {
	// Format: "N active connections" or "N active connections with listeners"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count - handle both "N active connections" and "N active connections with listeners"
	var count int
	if _, err := fmt.Sscanf(line, "%d active connections", &count); err != nil {
		return fmt.Errorf("parse active connections count from %q: %w", line, err)
	}

	// Skip connection data lines (one per connection)
	for i := 0; i < count; i++ {
		if _, err := r.ReadString('\n'); err != nil {
			return fmt.Errorf("read connection %d: %w", i, err)
		}
	}

	return nil
}
