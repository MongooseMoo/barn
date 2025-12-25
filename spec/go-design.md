# Go Implementation Design Notes

## Overview

This document describes Go-specific implementation decisions for the Barn MOO server.

---

## 1. Concurrency Model

### 1.1 MOO to Go Mapping

| MOO Concept | Go Implementation |
|-------------|-------------------|
| Task | Goroutine + Context |
| Task queue | Priority queue + scheduler goroutine |
| Fork | `go` + `time.After` |
| Suspend | Channel receive |
| Resume | Channel send |
| Kill task | Context cancellation |
| Tick counting | Instruction counter with periodic check |

### 1.2 Task Structure

```go
type Task struct {
    ID          int64
    State       TaskState
    VM          *VM
    Player      int64
    Programmer  int64
    StartTime   time.Time
    TicksUsed   int
    TickLimit   int
    Deadline    time.Time
    TaskLocal   sync.Map
    WakeChan    chan Value
    Ctx         context.Context
    Cancel      context.CancelFunc
}
```

### 1.3 Scheduler

```go
type Scheduler struct {
    waiting   *PriorityQueue  // Time-ordered
    running   *Task           // Currently executing
    mu        sync.Mutex
    wakeTimer *time.Timer
}

func (s *Scheduler) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-s.wakeTimer.C:
            s.runReadyTasks()
        }
    }
}
```

### 1.4 Suspension and Resume

```go
func (task *Task) Suspend(timeout time.Duration) Value {
    task.State = TaskSuspended

    var timer <-chan time.Time
    if timeout > 0 {
        timer = time.After(timeout)
    }

    select {
    case val := <-task.WakeChan:
        task.State = TaskRunning
        return val
    case <-timer:
        task.State = TaskRunning
        return IntValue(0)
    case <-task.Ctx.Done():
        return nil  // Task killed
    }
}
```

---

## 2. Type System

### 2.1 Value Interface

```go
type Value interface {
    Type() TypeCode
    String() string
    Equal(Value) bool
    Hash() uint64
    Clone() Value
}

type TypeCode int

const (
    TYPE_INT   TypeCode = 0
    TYPE_OBJ   TypeCode = 1
    TYPE_STR   TypeCode = 2
    TYPE_ERR   TypeCode = 3
    TYPE_LIST  TypeCode = 4
    TYPE_FLOAT TypeCode = 9
    TYPE_MAP   TypeCode = 10
    TYPE_BOOL  TypeCode = 11
    TYPE_WAIF  TypeCode = 12
)
```

### 2.2 Concrete Types

```go
type IntValue int64
type FloatValue float64
type BoolValue bool
type ObjValue int64
type ErrValue int

type StringValue struct {
    data string
    // 1-based indexing handled in methods
}

type ListValue struct {
    data []Value
    // Copy-on-write with reference counting
    refs *int32
}

type MapValue struct {
    entries map[uint64][]mapEntry
    // Hash-based with chaining for collisions
    refs *int32
}
```

### 2.3 Copy-on-Write

```go
func (l *ListValue) Set(index int, val Value) *ListValue {
    if atomic.LoadInt32(l.refs) > 1 {
        // Copy before write
        newData := make([]Value, len(l.data))
        copy(newData, l.data)
        atomic.AddInt32(l.refs, -1)
        return &ListValue{
            data: newData,
            refs: new(int32),
        }
    }
    l.data[index] = val
    return l
}
```

### 2.4 1-Based Indexing

```go
func (l *ListValue) Get(index int64) (Value, error) {
    // Convert 1-based to 0-based
    i := int(index) - 1
    if i < 0 || i >= len(l.data) {
        return nil, E_RANGE
    }
    return l.data[i], nil
}

func (s *StringValue) Substring(start, end int64) (string, error) {
    // MOO: 1-based, inclusive
    // Go: 0-based, exclusive
    if start < 1 || end < start {
        return "", E_RANGE
    }
    runes := []rune(s.data)
    if int(end) > len(runes) {
        return "", E_RANGE
    }
    return string(runes[start-1 : end]), nil
}
```

---

## 3. VM Architecture

### 3.1 VM Structure

```go
type VM struct {
    Stack     []Value      // Operand stack
    SP        int          // Stack pointer
    Frames    []StackFrame // Call stack
    FP        int          // Frame pointer
    Task      *Task        // Current task
    DB        *Database    // Object database
}

type StackFrame struct {
    Program     *Program
    IP          int
    BasePointer int
    Locals      []Value
    This        int64
    Player      int64
    Verb        string
    Caller      int64
    LoopStack   []LoopState
    ExceptStack []Handler
}
```

### 3.2 Execution Loop

```go
func (vm *VM) Run() (Value, error) {
    for {
        frame := &vm.Frames[vm.FP]
        if frame.IP >= len(frame.Program.Code) {
            break
        }

        op := OpCode(frame.Program.Code[frame.IP])
        frame.IP++

        if err := vm.Execute(op); err != nil {
            if !vm.HandleError(err) {
                return nil, err
            }
        }

        // Tick counting - check every N opcodes
        if countsTick(op) {
            vm.Task.TicksUsed++
            if vm.Task.TicksUsed >= vm.Task.TickLimit {
                return nil, E_TICKS
            }
        }

        // Periodic context check
        if vm.Task.TicksUsed%1000 == 0 {
            select {
            case <-vm.Task.Ctx.Done():
                return nil, E_KILLED
            default:
            }
        }
    }

    return vm.Pop(), nil
}
```

### 3.3 Opcode Dispatch

```go
func (vm *VM) Execute(op OpCode) error {
    switch op {
    case OP_PUSH:
        idx := vm.ReadByte()
        vm.Push(vm.CurrentFrame().Program.Constants[idx])

    case OP_ADD:
        b, a := vm.Pop(), vm.Pop()
        result, err := vm.Add(a, b)
        if err != nil {
            return err
        }
        vm.Push(result)

    case OP_CALL_BUILTIN:
        funcID := vm.ReadByte()
        argc := vm.ReadByte()
        args := vm.PopN(int(argc))
        result, err := vm.CallBuiltin(funcID, args)
        if err != nil {
            return err
        }
        vm.Push(result)

    // ... ~100 more cases
    }
    return nil
}
```

---

## 4. Database Layer

### 4.1 Object Storage

```go
type Database struct {
    objects    map[int64]*Object
    maxObject  int64
    recycled   []int64  // Recycled IDs for reuse
    mu         sync.RWMutex
}

type Object struct {
    ID        int64
    Name      string
    Owner     int64
    Parents   []int64
    Children  []int64
    Location  int64
    Contents  []int64
    Flags     ObjectFlags
    Props     map[string]*Property
    Verbs     map[string]*Verb
    mu        sync.RWMutex
}
```

### 4.2 Persistence

```go
// Checkpoint-based persistence
func (db *Database) Checkpoint(path string) error {
    db.mu.RLock()
    defer db.mu.RUnlock()

    f, err := os.Create(path + ".new")
    if err != nil {
        return err
    }
    defer f.Close()

    enc := gob.NewEncoder(f)
    for _, obj := range db.objects {
        if err := enc.Encode(obj); err != nil {
            return err
        }
    }

    f.Close()
    return os.Rename(path+".new", path)
}
```

### 4.3 Inheritance Resolution

```go
func (db *Database) FindProperty(objID int64, name string) (*Property, error) {
    visited := make(map[int64]bool)
    return db.findPropertyBFS(objID, name, visited)
}

func (db *Database) findPropertyBFS(objID int64, name string, visited map[int64]bool) (*Property, error) {
    queue := []int64{objID}

    for len(queue) > 0 {
        id := queue[0]
        queue = queue[1:]

        if visited[id] {
            continue
        }
        visited[id] = true

        obj := db.GetObject(id)
        if obj == nil {
            continue
        }

        if prop, ok := obj.Props[name]; ok && !prop.Clear {
            return prop, nil
        }

        queue = append(queue, obj.Parents...)
    }

    return nil, E_PROPNF
}
```

---

## 5. Connection Handling

### 5.1 Connection Manager

```go
type ConnectionManager struct {
    connections map[int64]*Connection
    listeners   map[int]*Listener
    mu          sync.RWMutex
}

type Connection struct {
    Player      int64
    Conn        net.Conn
    InputQueue  chan string
    OutputQueue chan string
    Options     ConnectionOptions
    ConnectedAt time.Time
    LastInput   time.Time
}
```

### 5.2 Input/Output

```go
func (c *Connection) ReadLoop() {
    reader := bufio.NewReader(c.Conn)
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            close(c.InputQueue)
            return
        }
        line = strings.TrimRight(line, "\r\n")
        c.LastInput = time.Now()
        c.InputQueue <- line
    }
}

func (c *Connection) WriteLoop() {
    for msg := range c.OutputQueue {
        c.Conn.Write([]byte(msg + "\r\n"))
    }
}
```

---

## 6. Builtin Functions

### 6.1 Registration

```go
type BuiltinFunc func(vm *VM, args []Value) (Value, error)

type BuiltinInfo struct {
    Name    string
    ID      int
    MinArgs int
    MaxArgs int  // -1 = variadic
    Func    BuiltinFunc
}

var builtins = []BuiltinInfo{
    {"length", 0, 1, 1, builtinLength},
    {"tostr", 1, 1, -1, builtinTostr},
    {"typeof", 2, 1, 1, builtinTypeof},
    // ... 165+ more
}
```

### 6.2 Example Implementation

```go
func builtinLength(vm *VM, args []Value) (Value, error) {
    switch v := args[0].(type) {
    case *StringValue:
        return IntValue(len([]rune(v.data))), nil
    case *ListValue:
        return IntValue(len(v.data)), nil
    case *MapValue:
        return IntValue(v.Len()), nil
    default:
        return nil, E_TYPE
    }
}
```

---

## 7. Error Handling

### 7.1 Error Types

```go
type MOOError struct {
    Code    ErrValue
    Message string
    Line    int
    Stack   []StackEntry
}

func (e *MOOError) Error() string {
    return fmt.Sprintf("%s: %s (line %d)", e.Code, e.Message, e.Line)
}
```

### 7.2 Error Propagation

```go
func (vm *VM) HandleError(err error) bool {
    mooErr, ok := err.(*MOOError)
    if !ok {
        return false
    }

    frame := &vm.Frames[vm.FP]

    // Check exception handlers
    for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
        handler := frame.ExceptStack[i]
        if handler.Matches(mooErr.Code) {
            frame.ExceptStack = frame.ExceptStack[:i]
            frame.IP = handler.HandlerIP
            if handler.VarIndex >= 0 {
                frame.Locals[handler.VarIndex] = mooErr.Code
            }
            return true
        }
    }

    // Propagate to caller
    if vm.FP > 0 {
        vm.FP--
        vm.SP = vm.Frames[vm.FP].BasePointer
        return vm.HandleError(err)
    }

    return false
}
```

---

## 8. Performance Considerations

### 8.1 Memory Pooling

```go
var valuePool = sync.Pool{
    New: func() any { return new(ListValue) },
}

var framePool = sync.Pool{
    New: func() any { return new(StackFrame) },
}
```

### 8.2 Stack Pre-allocation

```go
func NewVM() *VM {
    return &VM{
        Stack:  make([]Value, 0, 1024),
        Frames: make([]StackFrame, 0, 64),
    }
}
```

### 8.3 String Interning

```go
var stringIntern sync.Map

func InternString(s string) *StringValue {
    if v, ok := stringIntern.Load(s); ok {
        return v.(*StringValue)
    }
    sv := &StringValue{data: s}
    stringIntern.Store(s, sv)
    return sv
}
```

---

## 9. Testing Strategy

### 9.1 Conformance Tests

```go
func TestConformance(t *testing.T) {
    files, _ := filepath.Glob("../../cow_py/tests/conformance/**/*.yaml")
    for _, file := range files {
        suite := LoadTestSuite(file)
        for _, tc := range suite.Tests {
            t.Run(tc.Name, func(t *testing.T) {
                result := RunMOOCode(tc.Code)
                if tc.Expect.Value != nil {
                    assert.Equal(t, tc.Expect.Value, result)
                }
                if tc.Expect.Error != "" {
                    assert.ErrorIs(t, result, ParseError(tc.Expect.Error))
                }
            })
        }
    }
}
```

### 9.2 Benchmark Tests

```go
func BenchmarkListAppend(b *testing.B) {
    vm := NewVM()
    list := NewList(1000)
    val := IntValue(42)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        vm.ListAppend(list, val)
    }
}
```

---

## 10. Go Advantages Summary

| Feature | Go Advantage |
|---------|--------------|
| Concurrency | Goroutines map naturally to MOO tasks |
| Channels | Clean suspension/resumption |
| Context | Hierarchical task cancellation |
| Interfaces | Clean Value abstraction |
| Garbage collection | Handles MOO garbage collection |
| Type safety | Catch errors at compile time |
| Performance | Near-C performance when needed |
| Cross-compilation | Single binary deployment |
