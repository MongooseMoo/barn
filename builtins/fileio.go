package builtins

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

type mooFileHandle struct {
	id     int64
	file   *os.File
	name   string
	mode   string
	binary bool
}

func (h *mooFileHandle) canRead() bool {
	if len(h.mode) < 2 {
		return false
	}
	if h.mode[1] == '+' {
		return true // read-write mode
	}
	return h.mode[0] == 'r'
}

func (h *mooFileHandle) canWrite() bool {
	if len(h.mode) < 2 {
		return false
	}
	if h.mode[1] == '+' {
		return true // read-write mode
	}
	return h.mode[0] == 'w' || h.mode[0] == 'a'
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
	// Toast's file_resolve_path strips a leading "/" rather than rejecting it,
	// rooting the path inside files/ (so "/tmp/foo" → "files/tmp/foo").
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	// Toast's file_verify_path uses Unix semantics: only "/" separates path
	// components and a backslash is an ordinary filename character. Reject the
	// same forward-slash traversal patterns Toast rejects, but treat backslash
	// literally (so "foo\..\bar" is a plain — and almost certainly missing —
	// filename, yielding E_FILE rather than a traversal rejection).
	// Reject if it starts with "..".
	if len(path) > 1 && path[0] == '.' && path[1] == '.' {
		return "", fmt.Errorf("path traversal disallowed")
	}
	// Reject if it contains "/." anywhere (catches "../", "./", and hidden
	// files like ".hidden").
	if strings.Contains(path, "/.") {
		return "", fmt.Errorf("path traversal disallowed")
	}
	clean := filepath.Clean(path)
	// Defense in depth on platforms where "\" is also a path separator
	// (Windows): ensure the resolved path cannot escape the files/ root even
	// though we no longer reject backslashes outright.
	root, err := filepath.Abs("files")
	if err != nil {
		return "", err
	}
	full, err := filepath.Abs(filepath.Join("files", clean))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes files root")
	}
	return clean, nil
}

func parseFileOpenMode(mode string) (int, bool, error) {
	// Toast requires exactly 4 characters: [rwa][+-][tb][fn]
	if len(mode) != 4 {
		return 0, false, fmt.Errorf("invalid mode: must be exactly 4 characters")
	}
	// Position 0: r/w/a
	flags := 0
	switch mode[0] {
	case 'r':
		flags = os.O_RDONLY
	case 'w':
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	case 'a':
		flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	default:
		return 0, false, fmt.Errorf("invalid mode: first char must be r/w/a")
	}
	// Position 1: + or -
	switch mode[1] {
	case '+':
		if mode[0] == 'r' {
			flags = os.O_RDWR
		} else if mode[0] == 'w' {
			flags = os.O_CREATE | os.O_TRUNC | os.O_RDWR
		} else {
			flags = os.O_CREATE | os.O_APPEND | os.O_RDWR
		}
	case '-':
		// no change
	default:
		return 0, false, fmt.Errorf("invalid mode: second char must be + or -")
	}
	// Position 2: t or b
	binary := false
	switch mode[2] {
	case 't':
		// text mode
	case 'b':
		binary = true
	default:
		return 0, false, fmt.Errorf("invalid mode: third char must be t or b")
	}
	// Position 3: f or n
	switch mode[3] {
	case 'f', 'n':
		// flush or no-flush
	default:
		return 0, false, fmt.Errorf("invalid mode: fourth char must be f or n")
	}
	return flags, binary, nil
}

func getFileHandle(v types.Value) (*mooFileHandle, types.ErrorCode) {
	if v.Type() != types.TYPE_INT {
		return nil, types.E_TYPE
	}
	fileState.mu.Lock()
	defer fileState.mu.Unlock()
	handle := fileState.handles[v.Int()]
	if handle == nil {
		return nil, types.E_INVARG
	}
	return handle, types.E_NONE
}

// filterTextMode keeps only printable ASCII bytes (0x20-0x7E),
// matching Toast's raw_bytes_to_clean filter for text-mode reads.
func filterTextMode(data []byte) string {
	var b strings.Builder
	for _, c := range data {
		if (c >= 0x21 && c <= 0x7E) || c == 0x20 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func encodeBinaryBytes(data []byte) string {
	var b strings.Builder
	for _, ch := range data {
		encodeByte(&b, ch)
	}
	return b.String()
}

// EncodeRawToBinary converts raw bytes to MOO binary-string text (~XX for
// non-printables, ~7E for '~'), Toast's raw_bytes_to_binary. Capture
// boundaries that hand raw external bytes to MOO code (telnet OOB commands,
// exec output) must encode with this — MOO strings store the ENCODED text;
// toliteral emits string bytes raw and must not re-encode.
func EncodeRawToBinary(data []byte) string {
	return encodeBinaryBytes(data)
}

func builtinFileOpen(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR || args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0]
	mode := args[1]
	path, err := sanitizeFilePath(name.Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := ensureFilesRoot(); err != nil {
		return types.Err(types.E_FILE)
	}
	fullPath := resolveFilePath(path)
	flags, binary, err := parseFileOpenMode(mode.Str())
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
	fileState.handles[id] = &mooFileHandle{id: id, file: f, name: path, mode: mode.Str(), binary: binary}
	fileState.mu.Unlock()
	return types.Ok(types.NewInt(id))
}

func builtinFileClose(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileName(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileOpenmode(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileRead(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canRead() {
		return types.Err(types.E_INVARG)
	}
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	n := args[1].Int()
	if n < 0 {
		return types.Err(types.E_INVARG)
	}
	buf := make([]byte, n)
	count, err := h.file.Read(buf)
	if err != nil && err != io.EOF {
		return types.Err(types.E_FILE)
	}
	if count == 0 {
		return types.Err(types.E_FILE)
	}
	data := buf[:count]
	if h.binary {
		return types.Ok(types.NewStr(encodeBinaryBytes(data)))
	}
	return types.Ok(types.NewStr(filterTextMode(data)))
}

func builtinFileReadline(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canRead() {
		return types.Err(types.E_INVARG)
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
		return types.Err(types.E_FILE)
	}
	if h.binary {
		return types.Ok(types.NewStr(encodeBinaryBytes(buf)))
	}
	trimmed := bytes.TrimRight(buf, "\r\n")
	return types.Ok(types.NewStr(filterTextMode(trimmed)))
}

func builtinFileReadlines(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canRead() {
		return types.Err(types.E_INVARG)
	}
	if args[1].Type() != types.TYPE_INT || args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	start := args[1].Int()
	end := args[2].Int()
	if start < 1 || start > end {
		return types.Err(types.E_INVARG)
	}
	cur, _ := h.file.Seek(0, io.SeekCurrent)
	defer h.file.Seek(cur, io.SeekStart)
	if _, err := h.file.Seek(0, io.SeekStart); err != nil {
		return types.Err(types.E_FILE)
	}
	// Read line-by-line keeping the trailing '\n' (getline semantics), then
	// apply the handle's mode-dependent filter exactly like file_read /
	// file_readline do. Toast applies the file_type in_filter to the raw
	// getline() output (newline included): binary mode ~XX-encodes
	// non-printable bytes (incl. '\n' -> "~0A"), text mode drops them.
	// See ToastStunt src/fileio.cc bf_file_readlines (in_filter on line,len)
	// and src/utils.cc raw_bytes_to_binary / raw_bytes_to_clean.
	reader := bufio.NewReader(h.file)
	out := make([]types.Value, 0)
	lineNo := int64(0)
	for lineNo < end {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			if lineNo >= start {
				if h.binary {
					out = append(out, types.NewStr(encodeBinaryBytes(line)))
				} else {
					out = append(out, types.NewStr(filterTextMode(line)))
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return types.Err(types.E_FILE)
		}
	}
	return types.Ok(types.NewList(out))
}

func builtinFileWrite(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canWrite() {
		return types.Err(types.E_INVARG)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	s := args[1]
	var data []byte
	if h.binary {
		decoded, bad := decodeBinaryString(s.Str())
		if bad {
			return types.Err(types.E_INVARG)
		}
		data = decoded
	} else {
		data = []byte(s.Str())
	}
	n, err := h.file.Write(data)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(int64(n)))
}

func builtinFileWriteline(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canWrite() {
		return types.Err(types.E_INVARG)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	s := args[1]
	var data []byte
	if h.binary {
		decoded, bad := decodeBinaryString(s.Str())
		if bad {
			return types.Err(types.E_INVARG)
		}
		data = append(decoded, '\n')
	} else {
		data = []byte(s.Str() + "\n")
	}
	if _, err := h.file.Write(data); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileFlush(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	switch v.Type() {
	case types.TYPE_INT:
		w := v.Int()
		if w < 0 || w > 2 {
			return 0, types.E_INVARG
		}
		return int(w), types.E_NONE
	case types.TYPE_STR:
		s := strings.ToLower(strings.TrimSpace(v.Str()))
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

func builtinFileSeek(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	offset := args[1].Int()
	whence := io.SeekStart
	if len(args) == 3 {
		var code2 types.ErrorCode
		whence, code2 = parseSeekWhence(args[2])
		if code2 != types.E_NONE {
			return types.Err(code2)
		}
	}
	pos, err := h.file.Seek(offset, whence)
	if err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(pos))
}

func builtinFileTell(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileEOF(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	switch v.Type() {
	case types.TYPE_INT:
		h, code := getFileHandle(v)
		if code != types.E_NONE {
			return nil, code
		}
		st, err := h.file.Stat()
		if err != nil {
			return nil, types.E_FILE
		}
		return st, types.E_NONE
	case types.TYPE_STR:
		path, err := sanitizeFilePath(v.Str())
		if err != nil {
			return nil, types.E_INVARG
		}
		st, err := os.Stat(resolveFilePath(path))
		if err != nil {
			return nil, types.E_FILE
		}
		return st, types.E_NONE
	default:
		return nil, types.E_INVARG
	}
}

func builtinFileSize(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileMode(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileLastModify(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileLastAccess(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return builtinFileLastModify(ctx, args)
}

func builtinFileLastChange(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return builtinFileLastModify(ctx, args)
}

func builtinFileStat(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileType(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	st, code := fileStatFromValue(args[0])
	if code == types.E_FILE {
		return types.Ok(types.NewInt(0))
	}
	if code != types.E_NONE {
		return types.Err(code)
	}
	if st.IsDir() {
		return types.Ok(types.NewStr("directory"))
	}
	return types.Ok(types.NewStr("file"))
}

func builtinFileRemove(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Remove(resolveFilePath(path)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileRename(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR || args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	f, err1 := sanitizeFilePath(args[0].Str())
	t, err2 := sanitizeFilePath(args[1].Str())
	// Check dest first: invalid dest -> E_INVARG
	if err2 != nil {
		return types.Err(types.E_INVARG)
	}
	// Toast quirk: str_dup(file_resolve_path(bad_src)) does not propagate
	// NULL, so the rename proceeds and fails -> E_FILE
	if err1 != nil {
		return types.Err(types.E_FILE)
	}
	if err := os.Rename(resolveFilePath(f), resolveFilePath(t)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileMkdir(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := ensureFilesRoot(); err != nil {
		return types.Err(types.E_FILE)
	}
	mode := os.FileMode(0o755)
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		mode = os.FileMode(args[1].Int())
	}
	if err := os.Mkdir(resolveFilePath(path), mode); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileRmdir(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Remove(resolveFilePath(path)); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileChmod(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR || args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	// Toast validates mode string first, then path.
	// Mode must be exactly 3 octal digits (0-7).
	modeStr := args[1].Str()
	if len(modeStr) != 3 {
		return types.Err(types.E_INVARG)
	}
	var perm os.FileMode
	factor := os.FileMode(64) // 8^2 = 64
	for i := 0; i < 3; i++ {
		c := modeStr[i]
		if c < '0' || c > '7' {
			return types.Err(types.E_INVARG)
		}
		perm += factor * os.FileMode(c-'0')
		factor /= 8
	}
	path, err := sanitizeFilePath(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	if err := os.Chmod(resolveFilePath(path), perm); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinFileList(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	detailed := false
	if len(args) == 2 {
		detailed = args[1].Truthy()
	}
	path, err := sanitizeFilePath(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	entries, err := os.ReadDir(resolveFilePath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return types.Ok(types.NewList(nil))
		}
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

func builtinFileHandles(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinFileCountLines(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if !h.canRead() {
		return types.Err(types.E_INVARG)
	}
	if _, err := h.file.Seek(0, io.SeekStart); err != nil {
		return types.Err(types.E_FILE)
	}
	scanner := bufio.NewScanner(h.file)
	count := int64(0)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewInt(count))
}

func builtinFileGrep(ctx *kernel.TaskContext, args []types.Value) types.Result {
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
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 && args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if !h.canRead() {
		return types.Err(types.E_INVARG)
	}
	if _, err := h.file.Seek(0, io.SeekStart); err != nil {
		return types.Err(types.E_FILE)
	}
	matchAll := len(args) == 3 && args[2].Truthy()
	needle := args[1].Str()
	scanner := bufio.NewScanner(h.file)
	out := make([]types.Value, 0)
	lineNum := int64(0)
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if strings.Contains(line, needle) {
			out = append(out, types.NewList([]types.Value{
				types.NewStr(line),
				types.NewInt(lineNum),
			}))
			if !matchAll {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return types.Err(types.E_FILE)
	}
	return types.Ok(types.NewList(out))
}
