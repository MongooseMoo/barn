package store

import (
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// Keyed access to a map-valued property. A task that reads obj.prop[k] depends
// on the entry at k, not on the whole map, and a task that runs obj.prop[k] = v
// changes only that entry. Recording those dependencies per key lets two
// transactions that touch different keys of one shared map both commit: the
// reader validates the entries it read, and the writer's entries are applied to
// the map as it is at commit.
//
// Only accesses that the VM can see never let the map escape use this: a property
// read followed directly by an index, and an indexed assignment to a property.
// Every other read of the property is a whole read, recorded as before. A slot is
// eligible only when it holds a map directly on a numbered object (not clear,
// not inherited).

// mapKeyRead is one entry a transaction observed: the value at key, or that key
// was absent.
type mapKeyRead struct {
	key     types.Value
	value   types.Value
	present bool
}

// mapKeyState is what a transaction depends on and changed in one map-valued
// slot through keyed access.
type mapKeyState struct {
	// owner, perms and version are the slot as the transaction first saw it. The
	// permission checks depended on owner and perms, so a change to them conflicts;
	// version is what a whole read records if this state degrades.
	owner   types.ObjID
	perms   PropertyPerms
	version uint64
	name    string // original-case name
	reads   []mapKeyRead
	written []types.Value
	// baseline is the immutable map at the first eligible fetch, before any
	// keyed writes. It distinguishes snapshot entries from our own entries in
	// a captured map even when evaluating its index changes the slot again.
	baseline types.Value
	// pendingBase is the map a keyed-write base fetch returned and its
	// writeback has not consumed yet.
	pendingBase    types.Value
	hasPendingBase bool
	// sizeLimit is the smallest positive list-value limit a keyed write here
	// was checked against: a map of that many bytes or more is E_QUOTA. The
	// map built at commit is held to it. Zero means none.
	sizeLimit int
}

func (st *mapKeyState) wrote(key types.Value) bool {
	for _, k := range st.written {
		if types.MapKeysEqual(k, key) {
			return true
		}
	}
	return false
}

// keyedSlot returns the transaction's view of objID.name when keyed access may
// track it: a map held directly by a numbered object, with no whole read of the
// slot recorded yet. A staged non-keyed write disqualifies the slot too: the
// transaction then reads its own value and depends on nothing more.
func (tx *StoreTxn) keyedSlot(objID types.ObjID, name string) (string, Property, bool) {
	if tx == nil || tx.direct || tx.store == nil || tx.liveMutated {
		return "", Property{}, false
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) || obj.anonymous {
		return "", Property{}, false
	}
	actualName, prop, ok := obj.properties.find(name)
	if !ok || prop.clear || prop.value.Type() != types.TYPE_MAP {
		return "", Property{}, false
	}
	rkey := propertyReadKey{objID: objID, name: propertyNameKey(actualName)}
	if _, whole := tx.propertyReads[rkey]; whole {
		return "", Property{}, false
	}
	// A slot this transaction defined, or on an object it created, is its own:
	// reading it depends on nothing in the live store.
	if _, defined := tx.propertyDefines[propertyWriteKey(rkey)]; defined || tx.createdObjects[objID] != nil {
		return "", Property{}, false
	}
	st := tx.mapKeys[rkey]
	if _, staged := tx.propertyWrites[propertyWriteKey(rkey)]; staged && (st == nil || len(st.written) == 0) {
		return "", Property{}, false
	}
	return actualName, prop, true
}

func (tx *StoreTxn) mapKeyStateFor(objID types.ObjID, actualName string, prop Property) *mapKeyState {
	rkey := propertyReadKey{objID: objID, name: propertyNameKey(actualName)}
	st := tx.mapKeys[rkey]
	if st == nil {
		st = &mapKeyState{owner: prop.owner, perms: prop.perms, version: prop.version, name: actualName, baseline: prop.value}
		lazySet(&tx.mapKeys, rkey, st)
	}
	return st
}

// MapPropertyForKey resolves objID.name for a keyed read (forWrite false) or the
// base fetch of a keyed assignment (forWrite true). When the slot is eligible it
// records no whole read and reports keyed; the caller must then report the key
// it reads through ObserveMapKey, or write back through SetPropertyMapKey.
// Otherwise it is FindProperty and keyed is false.
func (tx *StoreTxn) MapPropertyForKey(objID types.ObjID, name string, forWrite bool) (PropertyView, bool, types.ErrorCode) {
	actualName, prop, ok := tx.keyedSlot(objID, name)
	if !ok {
		view, ec := tx.FindProperty(objID, name)
		return view, false, ec
	}
	st := tx.mapKeyStateFor(objID, actualName, prop)
	if forWrite {
		st.pendingBase, st.hasPendingBase = prop.value, true
	}
	return prop.View(actualName), true, types.E_NONE
}

// ObserveMapKey records the entry at key in the map a keyed fetch returned.
// The captured map precedes index evaluation, which may itself write the slot.
// Only an entry differing from the original snapshot is our own value and adds
// no dependency; the current written keys cannot classify an earlier capture.
func (tx *StoreTxn) ObserveMapKey(objID types.ObjID, name string, captured, key types.Value) {
	if tx == nil {
		return
	}
	st := tx.mapKeys[propertyReadKey{objID: objID, name: propertyNameKey(name)}]
	if st == nil {
		return
	}
	if isolationprobe.Fault("map-key-capture") && st.wrote(key) {
		return // restore the pre-fix classification after index side effects
	}
	value, present := captured.MapGet(key)
	original, originallyPresent := st.baseline.MapGet(key)
	if present != originallyPresent || (present && !value.Identical(original)) {
		return
	}
	for _, r := range st.reads {
		if types.MapKeysEqual(r.key, key) {
			return // the first observation is the dependency
		}
	}
	st.reads = append(st.reads, mapKeyRead{key: key, value: value, present: present})
}

// pendingKeyedBase returns the slot state when the keyed base fetched for an
// assignment is still the transaction's view of objID.name, so the map the
// assignment built differs from that view only at its key.
func (tx *StoreTxn) pendingKeyedBase(objID types.ObjID, name string) (string, Property, *mapKeyState, bool) {
	actualName, prop, ok := tx.keyedSlot(objID, name)
	if !ok {
		return "", Property{}, nil, false
	}
	st := tx.mapKeys[propertyReadKey{objID: objID, name: propertyNameKey(actualName)}]
	if st == nil || !st.hasPendingBase || !types.SameMapValue(st.pendingBase, prop.value) {
		if st != nil {
			st.hasPendingBase = false
		}
		return "", Property{}, nil, false
	}
	return actualName, prop, st, true
}

// KeyedWritebackView returns the slot an assignment's writeback will stage
// keyed, for its permission check, or false when the writeback is an ordinary
// property assignment. It records no read.
func (tx *StoreTxn) KeyedWritebackView(objID types.ObjID, name string) (PropertyView, bool) {
	actualName, prop, _, ok := tx.pendingKeyedBase(objID, name)
	if !ok {
		return PropertyView{}, false
	}
	return prop.View(actualName), true
}

// SetPropertyMapKey is the writeback of objID.name[key] = ...: next is the map the
// assignment built from the base MapPropertyForKey returned, already checked
// against sizeLimit (zero for none). It stays keyed when that base is still the
// transaction's view of the slot. Otherwise (the base was not keyed, was
// consumed, or the assignment's own evaluation changed the slot) it is an
// ordinary SetPropertyValue, which records a whole read.
func (tx *StoreTxn) SetPropertyMapKey(objID types.ObjID, name string, key, next types.Value, sizeLimit int) types.ErrorCode {
	actualName, prop, st, ok := tx.pendingKeyedBase(objID, name)
	if !ok || next.Type() != types.TYPE_MAP {
		return tx.SetPropertyValue(objID, name, next)
	}
	st.hasPendingBase = false
	if sizeLimit > 0 && (st.sizeLimit == 0 || sizeLimit < st.sizeLimit) {
		st.sizeLimit = sizeLimit
	}
	value, _ := next.MapGet(key)
	if current, present := prop.value.MapGet(key); present && current.Identical(value) && !st.wrote(key) {
		// Same-value elision, per key: the entry already holds value, so the
		// write is dropped and the transaction depends on that entry instead.
		tx.ObserveMapKey(objID, actualName, prop.value, key)
		if tx.store != nil {
			tx.store.propertyWriteElisions.Add(1)
		}
		return types.E_NONE
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	before := prop
	prop.value = next
	obj.properties.put(actualName, prop)
	if !st.wrote(key) {
		st.written = append(st.written, key)
	}
	tx.stagePropertyValue(objID, actualName, prop, next, before, true)
	if _, staged := tx.propertyWrites[propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}]; !staged {
		// The keyed writes net to the slot's original value and stagePropertyValue
		// dropped them. Their outcome depends on those entries now.
		for _, k := range st.written {
			v, present := next.MapGet(k)
			st.reads = append(st.reads, mapKeyRead{key: k, value: v, present: present})
		}
		st.written = nil
	}
	return types.E_NONE
}

// degradeMapKeys turns every keyed dependency into a whole read of its slot at
// the version the transaction first saw. A path that cannot validate per key or
// apply entries to the live map (live mutation, flush, the irreversible-effect
// boundary, renewal) then behaves exactly as it does for any property read.
func (tx *StoreTxn) degradeMapKeys() {
	for rkey, st := range tx.mapKeys {
		if _, whole := tx.propertyReads[rkey]; !whole {
			tx.propertyReads[rkey] = st.version
		}
	}
	tx.mapKeys = nil
}

// validateMapKeysLocked checks each keyed slot without a whole read: present, not
// clear, same owner and perms, still a map, and every observed entry unchanged.
func (tx *StoreTxn) validateMapKeysLocked(first *types.ErrorCode) {
	if isolationprobe.Fault("map-key-reads") {
		return
	}
	for rkey, st := range tx.mapKeys {
		if _, whole := tx.propertyReads[rkey]; whole {
			continue
		}
		c := ReadConflict{Kind: ConflictMapKey, ObjID: rkey.objID, Name: rkey.name, Read: st.version}
		errCode := types.E_INVARG
		live := tx.store.liveObjectLocked(rkey.objID)
		if !validLiveObject(live) {
			c.Missing, errCode = true, types.E_INVIND
			tx.noteConflict(first, errCode, c)
			continue
		}
		_, prop, ok := live.properties.find(rkey.name)
		switch {
		case !ok:
			c.Missing = true
		case prop.clear || prop.owner != st.owner || prop.perms != st.perms || prop.value.Type() != types.TYPE_MAP:
			c.Live = prop.version
		case !mapEntriesUnchanged(prop.value, st.reads):
			c.Live = prop.version
		default:
			continue
		}
		tx.noteConflict(first, errCode, c)
	}
}

func mapEntriesUnchanged(live types.Value, reads []mapKeyRead) bool {
	for _, r := range reads {
		v, present := live.MapGet(r.key)
		if present != r.present || (present && !v.Identical(r.value)) {
			return false
		}
	}
	return true
}

// rebaseMapKeyWritesLocked runs after a successful validation and before
// publication. For each slot written only through keyed assignments, the staged
// value becomes the live map with this transaction's entries set on it, so
// entries other commits changed since the snapshot survive. Validation has
// already established that the live slot is still a map with the same owner and
// perms. A map that would break the size limit its writes were checked against
// loses validation: run again, the assignment raises E_QUOTA where a serial
// execution would.
func (tx *StoreTxn) rebaseMapKeyWritesLocked() types.ErrorCode {
	if isolationprobe.Fault("map-key-rebase") {
		return types.E_NONE
	}
	for rkey, st := range tx.mapKeys {
		if len(st.written) == 0 {
			continue
		}
		if _, whole := tx.propertyReads[rkey]; whole {
			continue
		}
		wkey := propertyWriteKey(rkey)
		w, staged := tx.propertyWrites[wkey]
		if !staged {
			continue
		}
		live := tx.store.liveObjectLocked(rkey.objID)
		_, prop, ok := live.properties.find(rkey.name)
		if !ok || prop.value.Type() != types.TYPE_MAP {
			continue // unreachable after validation; publish as staged
		}
		if prop.version == st.version {
			continue // nothing else committed to the slot: the staged map is current
		}
		next := prop.value
		for _, k := range st.written {
			if v, present := w.value.MapGet(k); present {
				next = next.MapSet(k, v)
			}
		}
		if st.sizeLimit > 0 && types.ValueBytes(next) >= st.sizeLimit {
			errCode := types.E_NONE
			tx.noteConflict(&errCode, types.E_INVARG, ReadConflict{Kind: ConflictMapKey, ObjID: rkey.objID, Name: rkey.name, Read: st.version, Live: prop.version})
			return errCode
		}
		w.value, w.prop.value = next, next
		tx.propertyWrites[wkey] = w
	}
	return types.E_NONE
}
