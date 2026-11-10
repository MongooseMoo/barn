package bytecode

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/MongooseMoo/barn/types"
)

// Origin names the compile that produced a program, so a saved task can be
// rebuilt from its source instead of storing bytecode. The zero value is the
// compiler's program for the whole source (compiler.CompileMOO).
type Origin struct {
	// Fork is the one-based position, in code order, of this program's fork
	// instruction among all fork instructions of the program it was extracted
	// from transitively; 0 for a whole program and -1 for a range that is no
	// fork body.
	Fork int
	// Native marks a program compiled from one vector of a Toast checkpoint
	// (compiler.ConvertNativeFrame): Toast's NativeVector (-1 for its main
	// vector) of the source read as LanguageVersion.
	Native          bool
	NativeVector    int
	LanguageVersion int
}

// ExtractFork extracts fork body index (one-based, code order) of p, or nil
// when p has no such body.
func (p *Program) ExtractFork(index int) *Program {
	_, instructions, err := decodeInstructions(p.Code)
	if err != nil || index < 1 {
		return nil
	}
	for _, instruction := range instructions {
		if !isForkInstruction(instruction.op) {
			continue
		}
		if index--; index == 0 {
			_, length := forkOperands(instruction)
			return p.ExtractForkBody(instruction.next, length)
		}
	}
	return nil
}

// forkIndexAt returns the code-order position, counted from 1 within p, of
// the fork instruction whose body starts at bodyIP, or 0 when none does.
func (p *Program) forkIndexAt(bodyIP int) int {
	_, instructions, err := decodeInstructions(p.Code)
	if err != nil {
		return 0
	}
	position := 0
	for _, instruction := range instructions {
		if !isForkInstruction(instruction.op) {
			continue
		}
		position++
		if instruction.next == bodyIP {
			return position
		}
	}
	return 0
}

func isForkInstruction(op OpCode) bool {
	return op == OP_FORK || op == OP_FORK_WIDE || op == OP_FORK_LOCAL_WIDE
}

// Fingerprint hashes everything a frame running p depends on: code,
// constants, local names and count, line table, builtin layout and origin.
func (p *Program) Fingerprint() [32]byte {
	hash := sha256.New()
	write := func(parts ...any) {
		for _, part := range parts {
			switch value := part.(type) {
			case int:
				_ = binary.Write(hash, binary.BigEndian, int64(value))
			case bool:
				_ = binary.Write(hash, binary.BigEndian, value)
			case string:
				_ = binary.Write(hash, binary.BigEndian, int64(len(value)))
				hash.Write([]byte(value))
			case []byte:
				_ = binary.Write(hash, binary.BigEndian, int64(len(value)))
				hash.Write(value)
			}
		}
	}
	write(p.Code, len(p.Constants))
	for _, constant := range p.Constants {
		write(ConstantKey(constant))
	}
	write(len(p.VarNames))
	for _, name := range p.VarNames {
		write(name)
	}
	write(p.NumLocals, len(p.LineInfo))
	for _, entry := range p.LineInfo {
		write(entry.StartIP, entry.Line)
	}
	write(p.BuiltinLayout[:], p.Origin.Fork, p.Origin.Native, p.Origin.NativeVector, p.Origin.LanguageVersion)
	var sum [32]byte
	hash.Sum(sum[:0])
	return sum
}

// ConstantKey is the exact typed identity of a constant: the compiler
// deduplicates its pool by it, and fingerprints compare pools by it.
func ConstantKey(v types.Value) string {
	if v.Type() == types.TYPE_FLOAT {
		f := v.Float()
		if f == 0 {
			f = 0
		}
		return fmt.Sprintf("%d:%016x", int(v.Type()), math.Float64bits(f))
	}
	return fmt.Sprintf("%d:%s", int(v.Type()), v.String())
}
