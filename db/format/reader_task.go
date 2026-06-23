package format

import (
	"barn/types"
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// readFinalizations reads pending finalizations (v17)
func (database *Database) readFinalizations(r *bufio.Reader) error {
	// Format: "N values pending finalization"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	var count int
	if _, err := fmt.Sscanf(line, "%d values pending finalization", &count); err != nil {
		return fmt.Errorf("parse pending finalization count: %w", err)
	}

	database.PendingFinalizations = make([]types.Value, 0, count)
	for i := 0; i < count; i++ {
		val, err := database.readValue(r)
		if err != nil {
			return fmt.Errorf("read pending finalization %d: %w", i, err)
		}
		if objVal, ok := val.(types.ObjValue); ok && objVal.IsAnonymous() && objVal.ID() < 0 {
			continue
		}
		database.PendingFinalizations = append(database.PendingFinalizations, val)
	}
	return nil
}

// readClocks reads clocks section (obsolete)
func (database *Database) readClocks(r *bufio.Reader) error {
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
func (database *Database) readQueuedTasks(r *bufio.Reader) error {
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

	database.QueuedTasks = make([]*QueuedTask, 0, count)
	for i := 0; i < count; i++ {
		task, err := database.readQueuedTask(r)
		if err != nil {
			return fmt.Errorf("read queued task %d: %w", i, err)
		}
		database.QueuedTasks = append(database.QueuedTasks, task)
	}
	return nil
}

func (database *Database) readQueuedTask(r *bufio.Reader) (*QueuedTask, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read queued task header: %w", err)
	}

	var unused, firstLine int
	task := &QueuedTask{Variables: make(map[string]types.Value)}
	if _, err := fmt.Sscanf(header, "%d %d %d %d", &unused, &firstLine, &task.ID, &task.StartTime); err != nil {
		return nil, fmt.Errorf("parse queued task header %q: %w", strings.TrimSpace(header), err)
	}
	_ = unused
	_ = firstLine

	if err := database.readQueuedTaskActivation(r, task); err != nil {
		return nil, err
	}
	vars, err := database.readRtEnv(r)
	if err != nil {
		return nil, err
	}
	task.Variables = vars

	for {
		line, err := readLine(r)
		if err != nil {
			return nil, fmt.Errorf("read queued task source: %w", err)
		}
		if strings.TrimSpace(line) == "." {
			break
		}
		task.Code = append(task.Code, line)
	}

	return task, nil
}

func (database *Database) readQueuedTaskActivation(r *bufio.Reader, task *QueuedTask) error {
	if _, err := database.readValue(r); err != nil {
		return fmt.Errorf("read activation temp value: %w", err)
	}
	if tempThis, err := database.readValue(r); err != nil {
		return fmt.Errorf("read activation this value: %w", err)
	} else if obj, ok := tempThis.(types.ObjValue); ok {
		task.This = obj.ID()
	}
	if tempVerbLoc, err := database.readValue(r); err != nil {
		return fmt.Errorf("read activation verb location: %w", err)
	} else if obj, ok := tempVerbLoc.(types.ObjValue); ok {
		task.VerbLoc = obj.ID()
	}

	if _, err := r.ReadString('\n'); err != nil {
		return fmt.Errorf("read activation threaded flag: %w", err)
	}

	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read activation verbref: %w", err)
	}
	var thisObj, player, programmer, verbLoc types.ObjID
	var unused1, unused2, unused3, unused4, debug int
	if _, err := fmt.Sscanf(line, "%d %d %d %d %d %d %d %d %d",
		&thisObj, &unused1, &unused2, &player, &unused3, &programmer, &verbLoc, &unused4, &debug); err != nil {
		return fmt.Errorf("parse activation verbref %q: %w", strings.TrimSpace(line), err)
	}
	task.This = thisObj
	task.Player = player
	task.Programmer = programmer
	task.VerbLoc = verbLoc
	_ = debug

	for i := 0; i < 4; i++ {
		if _, err := readLine(r); err != nil {
			return fmt.Errorf("read activation placeholder %d: %w", i, err)
		}
	}

	verb, err := readLine(r)
	if err != nil {
		return fmt.Errorf("read activation verb: %w", err)
	}
	task.Verb = verb
	if _, err := readLine(r); err != nil {
		return fmt.Errorf("read activation verb aliases: %w", err)
	}

	return nil
}

func (database *Database) readRtEnv(r *bufio.Reader) (map[string]types.Value, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read runtime environment header: %w", err)
	}

	var count int
	if _, err := fmt.Sscanf(line, "%d variables", &count); err != nil {
		return nil, fmt.Errorf("parse runtime environment header %q: %w", strings.TrimSpace(line), err)
	}

	vars := make(map[string]types.Value, count)
	for i := 0; i < count; i++ {
		name, err := readLine(r)
		if err != nil {
			return nil, fmt.Errorf("read runtime variable %d name: %w", i, err)
		}
		value, err := database.readValue(r)
		if err != nil {
			return nil, fmt.Errorf("read runtime variable %q: %w", name, err)
		}
		vars[name] = value
	}
	return vars, nil
}

// readSuspendedTasks reads suspended tasks
func (database *Database) readSuspendedTasks(r *bufio.Reader) error {
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

	database.SuspendedTasks = make([]*SuspendedTask, 0, count)
	for i := 0; i < count; i++ {
		if err := database.skipSuspendedTask(r); err != nil {
			return fmt.Errorf("skip suspended task %d: %w", i, err)
		}
	}
	return nil
}

// skipSuspendedTask skips over a complete suspended task in the database file.
// A suspended task contains a VM with multiple activations (stack frames),
// each terminated by a period. We must parse the VM header to know how many
// activations to skip.
func (database *Database) skipSuspendedTask(r *bufio.Reader) error {
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
		if err := database.skipValueAfterType(r, typeCode); err != nil {
			return fmt.Errorf("read suspend value: %w", err)
		}
	}

	// Read VM local var
	if _, err := database.readValue(r); err != nil {
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
		if err := database.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// skipActivation skips over a single activation (stack frame) in a suspended task.
func (database *Database) skipActivation(r *bufio.Reader) error {
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
		if _, err := database.readValue(r); err != nil {
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
		if _, err := database.readValue(r); err != nil {
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
		if _, err := database.readValue(r); err != nil {
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
	if _, err := database.readValue(r); err != nil {
		return fmt.Errorf("read temp value: %w", err)
	}

	// PC info: "<pc> <bi_func_pc>[ <error_pc>]\n"
	pcLine, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read PC info: %w", err)
	}
	var pc, biFuncPC int
	if _, err := fmt.Sscanf(pcLine, "%d %d", &pc, &biFuncPC); err != nil {
		return fmt.Errorf("parse PC info %q: %w", strings.TrimSpace(pcLine), err)
	}
	_ = pc

	// If the activation is suspended inside a built-in function call (e.g.
	// eval(), move(), recycle()), Toast writes the function name and, for a
	// handful of functions that register read/write handlers, extra saved
	// state after the PC line.
	if biFuncPC != 0 {
		if err := database.skipBiFuncData(r); err != nil {
			return fmt.Errorf("skip bi_func data: %w", err)
		}
	}

	return nil
}

// skipBiFuncData skips the built-in-function name and, if applicable, the
// saved state written for it by Toast's write_bi_func_data.
func (database *Database) skipBiFuncData(r *bufio.Reader) error {
	name, err := readLine(r)
	if err != nil {
		return fmt.Errorf("read bi_func name: %w", err)
	}
	return database.skipBiFuncDataFor(r, strings.TrimSpace(name))
}

// skipBiFuncDataFor skips the saved state written by write_bi_func_data for
// the named built-in function. Only create, recreate, recycle, move, and
// call_function register read/write handlers (see functions.cc); every
// other built-in writes nothing extra. call_function nests recursively
// since it can itself be suspended while calling another built-in.
func (database *Database) skipBiFuncDataFor(r *bufio.Reader, name string) error {
	switch name {
	case "create", "recreate", "recycle", "move":
		if _, err := r.ReadString('\n'); err != nil {
			return fmt.Errorf("read %s bi_func data: %w", name, err)
		}
	case "call_function":
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read call_function bi_func data: %w", err)
		}
		const prefix = "bf_call_function data: fname = "
		inner := strings.TrimSpace(line)
		if strings.HasPrefix(inner, prefix) {
			inner = inner[len(prefix):]
		}
		return database.skipBiFuncDataFor(r, inner)
	}
	return nil
}

// readInterruptedTasks reads interrupted tasks
func (database *Database) readInterruptedTasks(r *bufio.Reader) error {
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
		if err := database.skipInterruptedTask(r); err != nil {
			return fmt.Errorf("skip interrupted task %d: %w", i, err)
		}
	}
	return nil
}

// skipInterruptedTask skips over a complete interrupted task.
// Format: "<task_id> <status_string>\n" followed by a VM.
func (database *Database) skipInterruptedTask(r *bufio.Reader) error {
	// Task header: "<task_id> <status_string>"
	// e.g., "1638619699 interrupted reading task"
	if _, err := r.ReadString('\n'); err != nil {
		return fmt.Errorf("read task header: %w", err)
	}

	// Read VM (same as suspended task VM, but no suspend value)
	// VM local var
	if _, err := database.readValue(r); err != nil {
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
		if err := database.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// readActiveConnections reads active connections
func (database *Database) readActiveConnections(r *bufio.Reader) error {
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
