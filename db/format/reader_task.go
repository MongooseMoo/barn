package format

import (
	"bufio"
	"fmt"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"strconv"
	"strings"
	"time"
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
		if val.Type() == types.TYPE_ANON && val.Obj() < 0 {
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
	if _, err := fmt.Sscanf(header, "%d %d %d %d", &unused, &firstLine, &task.StartTime, &task.ID); err != nil {
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
	} else if id, ok := asObjID(tempThis); ok {
		task.This = id
	}
	if tempVerbLoc, err := database.readValue(r); err != nil {
		return fmt.Errorf("read activation verb location: %w", err)
	} else if id, ok := asObjID(tempVerbLoc); ok {
		task.VerbLoc = id
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
		suspended, err := database.readSuspendedTask(r)
		if err != nil {
			return fmt.Errorf("read suspended task %d: %w", i, err)
		}
		database.SuspendedTasks = append(database.SuspendedTasks, suspended)
	}
	return nil
}

// skipSuspendedTask skips over a complete suspended task in the database file.
// A suspended task contains a VM with multiple activations (stack frames),
// each terminated by a period. We must parse the VM header to know how many
// activations to skip.
// skipActivation skips over a single activation (stack frame) in a suspended task.
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
		inner = strings.TrimPrefix(inner, prefix)
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

	for i := 0; i < count; i++ {
		interrupted, err := database.readInterruptedTask(r)
		if err != nil {
			return fmt.Errorf("read interrupted task %d: %w", i, err)
		}
		database.SuspendedTasks = append(database.SuspendedTasks, interrupted)
	}
	return nil
}

// readInterruptedTask reads a complete interrupted reading task.
// Format: "<task_id> <status_string>\n" followed by a VM.
func (database *Database) readInterruptedTask(r *bufio.Reader) (*SuspendedTask, error) {
	header, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("read task header: %w", err)
	}
	idText, status, found := strings.Cut(header, " ")
	if !found || strings.TrimSpace(status) == "" {
		return nil, fmt.Errorf("parse interrupted task header %q", header)
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse interrupted task id: %w", err)
	}

	taskLocal, err := database.readValue(r)
	if err != nil {
		return nil, fmt.Errorf("read task-local value: %w", err)
	}

	vmHeader, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("read VM header: %w", err)
	}
	var top, rootVector, functionID, maxStack int
	if n, scanErr := fmt.Sscanf(vmHeader, "%d %d %d %d", &top, &rootVector, &functionID, &maxStack); scanErr != nil || n != 4 {
		return nil, fmt.Errorf("parse interrupted VM header %q", vmHeader)
	}
	_ = rootVector
	_ = functionID
	if top < 0 {
		return nil, fmt.Errorf("invalid interrupted top activation %d", top)
	}

	vmSnapshot := &task.VMSnapshot{
		MaxStackDepth: maxStack,
		Frames:        make([]task.VMFrameSnapshot, 0, top+1),
	}
	callStack := make([]task.ActivationFrame, 0, top+1)
	for i := 0; i <= top; i++ {
		frame, activation, err := database.readVMFrame(r)
		if err != nil {
			return nil, fmt.Errorf("read activation %d: %w", i, err)
		}
		vmSnapshot.Frames = append(vmSnapshot.Frames, frame)
		callStack = append(callStack, activation)
	}

	root := callStack[0]
	return &SuspendedTask{Snapshot: task.Snapshot{
		ID:         id,
		Owner:      root.Player,
		State:      task.TaskQueued,
		StartTime:  time.Unix(0, 0),
		WakeValue:  types.NewErr(types.E_INTRPT),
		TaskLocal:  taskLocal,
		CallStack:  callStack,
		Programmer: root.Programmer,
		VerbLoc:    root.VerbLoc,
		VerbName:   root.Verb,
		This:       root.This,
		VM:         vmSnapshot,
	}}, nil
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

	withListeners := strings.Contains(line, "with listeners")
	database.ActiveConnections = make([]ActiveConnection, 0, count)
	for i := 0; i < count; i++ {
		connectionLine, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read connection %d: %w", i, err)
		}

		var player, listener int64
		if withListeners {
			if _, err := fmt.Sscanf(connectionLine, "%d %d", &player, &listener); err != nil {
				return fmt.Errorf("parse connection %d from %q: %w", i, connectionLine, err)
			}
		} else {
			if _, err := fmt.Sscanf(connectionLine, "%d", &player); err != nil {
				return fmt.Errorf("parse connection %d from %q: %w", i, connectionLine, err)
			}
		}
		database.ActiveConnections = append(database.ActiveConnections, ActiveConnection{
			Player:   types.ObjID(player),
			Listener: types.ObjID(listener),
		})
	}

	return nil
}
