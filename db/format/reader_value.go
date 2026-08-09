package format

import (
	"bufio"
	"fmt"
	"github.com/MongooseMoo/barn/types"
	"strconv"
	"strings"
)

// readValue reads a MOO value from database format
func (database *Database) readValue(r *bufio.Reader) (types.Value, error) {
	typeCode, err := readInt(r)
	if err != nil {
		return types.None, err
	}
	return database.readValueAfterType(r, typeCode)
}

func (database *Database) readValueAfterType(r *bufio.Reader, typeCode int) (types.Value, error) {
	version := database.Version
	switch typeCode {
	case 0: // INT
		val, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewInt(int64(val)), nil

	case 1: // OBJ
		objID, err := readObjID(r)
		if err != nil {
			return types.None, err
		}
		return types.NewObj(objID), nil

	case 2: // STR
		line, err := r.ReadString('\n')
		if err != nil {
			return types.None, err
		}
		return types.NewStr(strings.TrimRight(line, "\n\r")), nil

	case 3: // ERR
		errCode, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewErr(types.ErrorCode(errCode)), nil

	case 4: // LIST
		count, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		elements := make([]types.Value, count)
		for i := 0; i < count; i++ {
			elements[i], err = database.readValue(r)
			if err != nil {
				return types.None, err
			}
		}
		return types.NewList(elements), nil

	case 5: // CLEAR
		return types.None, nil // Clear property marker

	case 6: // NONE
		return types.NewInt(0), nil // None becomes 0

	case 7: // CATCH (stack marker)
		val, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewInt(int64(val)), nil

	case 8: // FINALLY (stack marker)
		val, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewInt(int64(val)), nil

	case 9: // FLOAT
		line, err := r.ReadString('\n')
		if err != nil {
			return types.None, err
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err != nil {
			return types.None, err
		}
		return types.NewFloat(val), nil

	case 10: // MAP (v17)
		if version < 17 {
			return types.None, fmt.Errorf("MAP type requires version 17+")
		}
		count, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		pairs := make([][2]types.Value, count)
		for i := 0; i < count; i++ {
			key, err := database.readValue(r)
			if err != nil {
				return types.None, err
			}
			val, err := database.readValue(r)
			if err != nil {
				return types.None, err
			}
			pairs[i] = [2]types.Value{key, val}
		}
		return types.NewMap(pairs), nil

	case 12: // ANON (anonymous object)
		// Just read the object ID
		objID, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewAnon(types.ObjID(objID)), nil

	case 13: // WAIF
		// WAIFs are saved as references ('r') or creations ('c').
		// Format: "{marker} {index}\n" where marker is 'r' or 'c'.
		line, err := r.ReadString('\n')
		if err != nil {
			return types.None, err
		}
		line = strings.TrimSpace(line)
		if len(line) < 1 {
			return types.None, fmt.Errorf("empty WAIF marker")
		}
		marker := line[0]

		if marker == 'r' {
			// Reference to previously saved WAIF.
			// Read the "." terminator line.
			if _, err := r.ReadString('\n'); err != nil {
				return types.None, err
			}
			// Parse index from marker line: "r {index}"
			refIdx, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err != nil {
				return types.None, fmt.Errorf("parse WAIF ref index: %w", err)
			}
			if refIdx < 0 || refIdx >= len(database.savedWaifs) {
				return types.None, fmt.Errorf("WAIF ref index %d out of range (have %d)", refIdx, len(database.savedWaifs))
			}
			return database.savedWaifs[refIdx].waif, nil

		} else if marker == 'c' {
			// Creation — read full WAIF structure.
			class, err := readObjID(r)
			if err != nil {
				return types.None, err
			}
			owner, err := readObjID(r)
			if err != nil {
				return types.None, err
			}
			// propdefs_length (needed for sparse property storage)
			if _, err := readInt(r); err != nil {
				return types.None, err
			}

			// Register the WAIF in savedWaifs BEFORE reading properties.
			// Properties may contain references back to this WAIF (or to
			// WAIFs nested inside it), so it must be findable by index.
			// This matches Toast's order: saved_waifs[waif_count++] = w,
			// then read properties.
			waif := types.NewWaif(class, owner)
			wIdx := len(database.savedWaifs)
			database.savedWaifs = append(database.savedWaifs, waifLoadData{
				waif: waif,
			})

			// Read property index→value pairs until -1 sentinel.
			propsByIndex := make(map[int]types.Value)
			for {
				propIdx, err := readInt(r)
				if err != nil {
					return types.None, err
				}
				if propIdx < 0 {
					break
				}
				val, err := database.readValue(r)
				if err != nil {
					return types.None, err
				}
				propsByIndex[propIdx] = val
			}
			// Read the "." terminator line.
			if _, err := r.ReadString('\n'); err != nil {
				return types.None, fmt.Errorf("read WAIF terminator: %w", err)
			}

			// Update with the properties now that they're loaded.
			database.savedWaifs[wIdx] = waifLoadData{
				waif:         waif,
				propsByIndex: propsByIndex,
			}
			return waif, nil
		}
		return types.None, fmt.Errorf("unknown WAIF marker: %c", marker)

	case 14: // BOOL (v17)
		if version < 17 {
			return types.None, fmt.Errorf("BOOL type requires version 17+")
		}
		val, err := readInt(r)
		if err != nil {
			return types.None, err
		}
		return types.NewBool(val != 0), nil

	default:
		return types.None, fmt.Errorf("unsupported type code: %d", typeCode)
	}
}

// skipValueAfterType consumes a value when the type code is already known.
// Used when the type code appears on the same line as other data. Keep this as
// a thin wrapper around the value reader so the two paths cannot disagree about
// the database wire format.
func (database *Database) skipValueAfterType(r *bufio.Reader, typeCode int) error {
	_, err := database.readValueAfterType(r, typeCode)
	return err
}
