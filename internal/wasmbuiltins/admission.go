package wasmbuiltins

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Section envelopes and allocation-driving declarations are inspected here.
// The standard WIT parser and runtime validate types and instructions; this
// fences allocations that a per-linear-memory limit does not cover.
func coreAllocations(data []byte) error {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{0, 97, 115, 109, 1, 0, 0, 0}) {
		return errors.New("invalid embedded core module")
	}
	locals := uint64(0)
	for pos := 8; pos < len(data); {
		id := data[pos]
		pos++
		n, size := binary.Uvarint(data[pos:])
		if size <= 0 || size > 5 || n > uint64(len(data)-pos-size) {
			return errors.New("invalid core section")
		}
		pos += size
		section := data[pos : pos+int(n)]
		pos += int(n)
		switch id {
		case 1, 2, 3, 5, 6, 7, 9, 10, 11, 13:
			count, used := binary.Uvarint(section)
			if used <= 0 || used > 5 || count > 4096 || count > uint64(len(section)-used) {
				return errors.New("core section vector exceeds admission limits")
			}
			if id == 5 && count > 1 {
				return errors.New("multiple core memories are not supported")
			}
		}
		if id == 1 {
			if err := coreFunctionTypes(section); err != nil {
				return err
			}
		}
		if id == 10 {
			if err := coreFunctionBodies(section, &locals); err != nil {
				return err
			}
		}
		if id != 4 {
			continue
		}
		count, used := binary.Uvarint(section)
		if used <= 0 || used > 5 || count > 1 {
			return errors.New("at most one bounded core table is supported")
		}
		section = section[used:]
		for range count {
			if len(section) < 2 || section[0] != 0x70 || section[1] != 1 {
				return errors.New("core tables require funcref and an explicit maximum")
			}
			section = section[2:]
			minimum, used := binary.Uvarint(section)
			if used <= 0 || used > 5 {
				return errors.New("invalid core table minimum")
			}
			section = section[used:]
			maximum, used := binary.Uvarint(section)
			if used <= 0 || used > 5 || minimum > maximum || maximum > 16384 {
				return errors.New("core table exceeds 16384 elements")
			}
			section = section[used:]
		}
		if len(section) != 0 {
			return errors.New("invalid core table section")
		}
	}
	return nil
}

type cursor struct {
	data []byte
	pos  int
}

func coreFunctionTypes(data []byte) error {
	c := cursor{data: data}
	count, err := c.index()
	if err != nil {
		return err
	}
	for range count {
		kind, err := c.byte()
		if err != nil || kind != 0x60 {
			return errors.New("only core function types are supported")
		}
		for range 2 { // Parameter and result vectors.
			n, err := c.index()
			if err != nil || n > 64 {
				return errors.New("core function type exceeds 64 parameters or results")
			}
			for range n {
				typ, err := c.byte()
				if err != nil {
					return err
				}
				switch typ {
				case 0x7f, 0x7e, 0x7d, 0x7c, 0x7b, 0x70, 0x6f:
				default:
					return errors.New("unsupported core value type")
				}
			}
		}
	}
	if c.pos != len(data) {
		return errors.New("invalid core type section")
	}
	return nil
}

func coreFunctionBodies(data []byte, total *uint64) error {
	c := cursor{data: data}
	count, err := c.index()
	if err != nil {
		return err
	}
	for range count {
		n, err := c.index()
		if err != nil || n > uint64(len(data)-c.pos) {
			return errors.New("invalid core function body")
		}
		body := cursor{data: data[c.pos : c.pos+int(n)]}
		c.pos += int(n)
		groups, err := body.index()
		if err != nil || groups > 4096 || groups > uint64(len(body.data)-body.pos) {
			return errors.New("core local groups exceed admission limits")
		}
		locals := uint64(0)
		for range groups {
			n, err := body.index()
			if err != nil {
				return err
			}
			locals += n
			if locals > 4096 || *total+locals > 65536 {
				return errors.New("core locals exceed admission limits")
			}
			if _, err := body.byte(); err != nil {
				return err
			}
		}
		*total += locals
	}
	if c.pos != len(data) {
		return errors.New("invalid core code section")
	}
	return nil
}

func (c *cursor) byte() (byte, error) {
	if c.pos == len(c.data) {
		return 0, errors.New("truncated WASM section")
	}
	b := c.data[c.pos]
	c.pos++
	return b, nil
}
func (c *cursor) index() (uint64, error) {
	n, size := binary.Uvarint(c.data[c.pos:])
	if size <= 0 || size > 5 || n > 1<<32-1 {
		return 0, errors.New("invalid WASM index")
	}
	c.pos += size
	return n, nil
}
func canonicalLifts(data []byte) (uint64, error) {
	c := cursor{data: data}
	count, err := c.index()
	if err != nil || count > 64 {
		return 0, errors.New("canonical function limit exceeded")
	}
	for range count {
		kind, err := c.byte()
		if err != nil || kind != 0 {
			return 0, errors.New("only synchronous canonical lifts are supported")
		}
		prefix, err := c.byte()
		if err != nil || prefix != 0 {
			return 0, errors.New("invalid canonical lift")
		}
		if _, err = c.index(); err != nil {
			return 0, err
		}
		n, err := c.index()
		if err != nil || n > 4 {
			return 0, errors.New("canonical option limit exceeded")
		}
		seen := make(map[byte]bool)
		for range n {
			opt, err := c.byte()
			if err != nil {
				return 0, err
			}
			if seen[opt] {
				return 0, errors.New("duplicate canonical option")
			}
			seen[opt] = true
			switch opt {
			case 0: // UTF-8 (the implicit default is also UTF-8).
			case 3, 4, 5:
				if _, err = c.index(); err != nil {
					return 0, err
				}
			default:
				return 0, errors.New("only UTF-8 synchronous canonical options are supported")
			}
		}
		if _, err = c.index(); err != nil {
			return 0, err
		}
	}
	if c.pos != len(data) {
		return 0, errors.New("invalid canonical section length")
	}
	return count, nil
}
