package builtins

import (
	"barn/types"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type mooFileHandle struct {
	id     int64
	file   *os.File
	name   string
	mode   string
	binary bool
}

var fileState = struct {
	mu      sync.Mutex
	nextID  int64
	handles map[int64]*mooFileHandle
}{
	nextID:  1,
	handles: make(map[int64]*mooFileHandle),
}

func resolveFilePath(rel string) string {
	return filepath.Join("files", rel)
}

func ensureFilesRoot() error {
	return os.MkdirAll("files", 0o755)
}

func sanitizeFilePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path disallowed")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal disallowed")
	}
	return clean, nil
}

func parseFileOpenMode(mode string) (int, bool, error) {
	if mode == "" {
		return 0, false, fmt.Errorf("empty mode")
	}
	binary := strings.Contains(mode, "b")
	flags := 0
	switch mode[0] {
	case 'r':
		flags = os.O_RDONLY
		if strings.Contains(mode, "+") {
			flags = os.O_RDWR
		}
	case 'w':
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		if strings.Contains(mode, "+") {
			flags = os.O_CREATE | os.O_TRUNC | os.O_RDWR
		}
	case 'a':
		flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
		if strings.Contains(mode, "+") {
			flags = os.O_CREATE | os.O_APPEND | os.O_RDWR
		}
	default:
		return 0, false, fmt.Errorf("invalid mode")
	}
	return flags, binary, nil
}

func getFileHandle(v types.Value) (*mooFileHandle, types.ErrorCode) {
	h, ok := v.(types.IntValue)
	if !ok {
		return nil, types.E_TYPE
	}
	fileState.mu.Lock()
	defer fileState.mu.Unlock()
	handle := fileState.handles[h.Val]
	if handle == nil {
		return nil, types.E_INVARG
	}
	return handle, types.E_NONE
}

func encodeBinaryBytes(data []byte) string {
	var b strings.Builder
	for _, ch := range data {
		encodeByte(&b, ch)
	}
	return b.String()
}

func builtinFileOpen(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	name, ok1 := args[0].(types.StrValue)
	mode, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(name.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := ensureFilesRoot(); err != nil {
		return types.Err(types.E_FILE)
	}
	fullPath := resolveFilePath(path)
	flags, binary, err := parseFileOpenMode(mode.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	f, err := os.OpenFile(fullPath, flags, 0o666)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	fileState.mu.Lock()
	id := fileState.nextID
	fileState.nextID++
	fileState.handles[id] = &mooFileHandle{id: id, file: f, name: path, mode: mode.Value(), binary: binary}
	fileState.mu.Unlock()
	return types.Ok(types.NewInt(id))
}

func builtinFileClose(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	_ = h.file.Close()
	fileState.mu.Lock()
	delete(fileState.handles, h.id)
	fileState.mu.Unlock()
	return types.Ok(types.NewInt(0))
}

func builtinFileName(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewStr(h.name))
}

func builtinFileOpenmode(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewStr(h.mode))
}

func builtinFileRead(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	n, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if n.Val < 0 {
		return types.Err(types.E_INVARG)
	}
	buf := make([]byte, n.Val)
	count, err := h.file.Read(buf)
	if err != nil && err != io.EOF {
		return types.Err(types.E_FILE)
	}
	data := buf[:count]
	if h.binary {
		return types.Ok(types.NewStr(encodeBinaryBytes(data)))
	}
	return types.Ok(types.NewStr(strings.ReplaceAll(string(data), "\r\n", "\n")))
}

func builtinFileReadline(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	var buf []byte
	tmp := make([]byte, 1)
	for {
		n, err := h.file.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[0])
			if tmp[0] == '\n' {
				break
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return types.Err(types.E_FILE)
		}
	}
	if len(buf) == 0 {
		return types.Ok(types.NewStr(""))
	}
	if h.binary {
		return types.Ok(types.NewStr(encodeBinaryBytes(buf)))
	}
	line := strings.TrimRight(strings.ReplaceAll(string(buf), "\r\n", "\n"), "\n")
	return types.Ok(types.NewStr(line))
}

func builtinFileReadlines(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	start, ok1 := args[1].(types.IntValue)
	count, ok2 := args[2].(types.IntValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	if start.Val < 1 || count.Val < 0 {
		return types.Err(types.E_INVARG)
	}
	cur, _ := h.file.Seek(0, io.SeekCurrent)
	defer h.file.Seek(cur, io.SeekStart)
	if _, err := h.file.Seek(0, io.SeekStart); err != nil {
		return types.Err(types.E_FILE)
	}
	scanner := bufio.NewScanner(h.file)
	out := make([]types.Value, 0)
	lineNo := int64(0)
	for scanner.Scan() {
		lineNo++
		if lineNo < start.Val {
			continue
		}
		if count.Val > 0 && int64(len(out)) >= count.Val {
			break
		}
		out = append(out, types.NewStr(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewList(out))
}

func builtinFileWrite(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	s, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	var data []byte
	if h.binary {
		decoded, bad := decodeBinaryString(s.Value())
		if bad {
			return types.Err(types.E_INVARG)
		}
		data = decoded
	} else {
		data = []byte(s.Value())
	}
	n, err := h.file.Write(data)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(int64(n)))
}

func builtinFileWriteline(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	s, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if _, err := h.file.WriteString(s.Value() + "\n"); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileFlush(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	if err := h.file.Sync(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func parseSeekWhence(v types.Value) (int, types.ErrorCode) {
	switch w := v.(type) {
	case types.IntValue:
		if w.Val < 0 || w.Val > 2 {
			return 0, types.E_INVARG
		}
		return int(w.Val), types.E_NONE
	case types.StrValue:
		s := strings.ToLower(strings.TrimSpace(w.Value()))
		switch s {
		case "", "set", "start", "seek_set":
			return io.SeekStart, types.E_NONE
		case "cur", "current", "seek_cur":
			return io.SeekCurrent, types.E_NONE
		case "end", "seek_end":
			return io.SeekEnd, types.E_NONE
		default:
			return 0, types.E_INVARG
		}
	default:
		return 0, types.E_TYPE
	}
}

func builtinFileSeek(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	offset, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	whence := io.SeekStart
	if len(args) == 3 {
		var code2 types.ErrorCode
		whence, code2 = parseSeekWhence(args[2])
		if code2 != types.E_NONE {
			return types.Err(code2)
		}
	}
	pos, err := h.file.Seek(offset.Val, whence)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(pos))
}

func builtinFileTell(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	pos, err := h.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(pos))
}

func builtinFileEOF(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getFileHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	pos, err := h.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	st, err := h.file.Stat()
	if err != nil {
		return types.Err(types.E_FILE)
	}
	if pos >= st.Size() {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

func fileStatFromValue(v types.Value) (os.FileInfo, types.ErrorCode) {
	switch x := v.(type) {
	case types.IntValue:
		h, code := getFileHandle(x)
		if code != types.E_NONE {
			return nil, code
		}
		st, err := h.file.Stat()
		if err != nil {
			return nil, types.E_FILE
		}
		return st, types.E_NONE
	case types.StrValue:
		path, err := sanitizeFilePath(x.Value())
		if err != nil {
			return nil, types.E_INVARG
		}
		st, err := os.Stat(resolveFilePath(path))
		if err != nil {
			return nil, types.E_FILE
		}
		return st, types.E_NONE
	default:
		return nil, types.E_TYPE
	}
}

func builtinFileSize(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	st, code := fileStatFromValue(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewInt(st.Size()))
}

func builtinFileMode(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	st, code := fileStatFromValue(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewInt(int64(st.Mode().Perm())))
}

func builtinFileLastModify(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	st, code := fileStatFromValue(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewInt(st.ModTime().Unix()))
}

func builtinFileLastAccess(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return builtinFileLastModify(ctx, args)
}

func builtinFileLastChange(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return builtinFileLastModify(ctx, args)
}

func builtinFileStat(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(s.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	st, err := os.Stat(resolveFilePath(path))
	if err != nil {
		return types.Err(types.E_FILE)
	}
	kind := "reg"
	if st.IsDir() {
		kind = "dir"
	}
	return types.Ok(types.NewList([]types.Value{
		types.NewStr(st.Name()),
		types.NewStr(kind),
		types.NewInt(st.Size()),
		types.NewInt(int64(st.Mode().Perm())),
		types.NewInt(st.ModTime().Unix()),
	}))
}

func builtinFileType(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(s.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	st, err := os.Stat(resolveFilePath(path))
	if err != nil {
		return types.Ok(types.NewInt(0))
	}
	if st.IsDir() {
		return types.Ok(types.NewStr("directory"))
	}
	return types.Ok(types.NewStr("file"))
}

func builtinFileRemove(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(s.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Remove(resolveFilePath(path)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileRename(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	from, ok1 := args[0].(types.StrValue)
	to, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	f, err1 := sanitizeFilePath(from.Value())
	t, err2 := sanitizeFilePath(to.Value())
	if err1 != nil || err2 != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Rename(resolveFilePath(f), resolveFilePath(t)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileMkdir(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := ensureFilesRoot(); err != nil {
		return types.Err(types.E_FILE)
	}
	mode := os.FileMode(0o755)
	if len(args) == 2 {
		perm, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		mode = os.FileMode(perm.Val)
	}
	if err := os.Mkdir(resolveFilePath(path), mode); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileRmdir(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Remove(resolveFilePath(path)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileChmod(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok1 := args[0].(types.StrValue)
	permVal, ok2 := args[1].(types.IntValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Chmod(resolveFilePath(path), os.FileMode(permVal.Val)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileList(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	detailed := false
	if len(args) == 2 {
		detailed = args[1].Truthy()
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	entries, err := os.ReadDir(resolveFilePath(path))
	if err != nil {
		return types.Err(types.E_FILE)
	}
	out := make([]types.Value, 0, len(entries))
	for _, e := range entries {
		if detailed {
			kind := "file"
			if e.IsDir() {
				kind = "directory"
			}
			out = append(out, types.NewMap([][2]types.Value{
				{types.NewStr("name"), types.NewStr(e.Name())},
				{types.NewStr("type"), types.NewStr(kind)},
			}))
		} else {
			out = append(out, types.NewStr(e.Name()))
		}
	}
	return types.Ok(types.NewList(out))
}

func builtinFileHandles(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	fileState.mu.Lock()
	ids := make([]int64, 0, len(fileState.handles))
	for id := range fileState.handles {
		ids = append(ids, id)
	}
	fileState.mu.Unlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.NewInt(id))
	}
	return types.Ok(types.NewList(out))
}

func builtinFileCountLines(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(s.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	f, err := os.Open(resolveFilePath(path))
	if err != nil {
		return types.Err(types.E_FILE)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := int64(0)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(count))
}

func builtinFileGrep(ctx *types.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok1 := args[0].(types.StrValue)
	patVal, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	re, err := regexp.Compile(patVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	f, err := os.Open(resolveFilePath(path))
	if err != nil {
		return types.Err(types.E_FILE)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	out := make([]types.Value, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			out = append(out, types.NewStr(line))
		}
	}
	if err := scanner.Err(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewList(out))
}
