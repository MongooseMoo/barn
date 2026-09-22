package store

import "github.com/MongooseMoo/barn/types"

type waifTxnImage struct {
	value  types.Value
	base   *types.WaifImage
	staged map[string]types.Value
}

func (tx *StoreTxn) waifImageLocked(value types.Value) (*waifTxnImage, types.ErrorCode) {
	if tx == nil || tx.store == nil || value.Type() != types.TYPE_WAIF {
		return nil, types.E_INVARG
	}
	if !tx.direct {
		if image := tx.waifs[value.WaifIdentity()]; image != nil {
			return image, types.E_NONE
		}
	}
	ts := tx.readTS
	if tx.direct {
		ts = tx.store.readTimestamp()
	}
	base, ok := value.WaifImageAt(tx.store.waifDomain, ts)
	if !ok {
		return nil, types.E_INVARG
	}
	image := &waifTxnImage{value: value, base: base}
	if !tx.direct {
		lazySet(&tx.waifs, value.WaifIdentity(), image)
	}
	return image, types.E_NONE
}

// WaifProperty reads the task's private image, including absence dependencies.
func (tx *StoreTxn) WaifProperty(value types.Value, name string) (types.Value, bool, types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return types.Value{}, false, types.E_INVARG
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return types.Value{}, false, ec
	}
	if image.staged != nil {
		v, ok := image.staged[name]
		return v, ok, types.E_NONE
	}
	v, ok := image.base.Property(name)
	return v, ok, types.E_NONE
}

// WaifProperties supplies a detached property map for recursive value walks.
func (tx *StoreTxn) WaifProperties(value types.Value) (map[string]types.Value, types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return nil, types.E_INVARG
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return nil, ec
	}
	properties := image.base.Properties()
	if image.staged != nil {
		for name, value := range image.staged {
			properties[name] = value
		}
	}
	return properties, types.E_NONE
}

func (tx *StoreTxn) SetWaifProperty(value types.Value, name string, next types.Value) types.ErrorCode {
	if tx == nil || tx.store == nil {
		return types.E_INVARG
	}
	if tx.direct {
		tx.store.mu.Lock()
		defer tx.store.mu.Unlock()
	} else {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
	}
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return ec
	}
	if image.staged == nil {
		image.staged = image.base.Properties()
	}
	image.staged[name] = next
	if tx.direct {
		tx.store.publishWaifLocked(image, tx.store.bumpClockLocked())
	}
	return types.E_NONE
}

func (tx *StoreTxn) hasWaifWrites() bool {
	for _, image := range tx.waifs {
		if image.staged != nil {
			return true
		}
	}
	return false
}

func (tx *StoreTxn) validateWaifsLocked() types.ErrorCode {
	for _, image := range tx.waifs {
		live, ok := image.value.WaifImageAt(tx.store.waifDomain, tx.store.readTimestamp())
		if !ok || live.Timestamp() != image.base.Timestamp() {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (s *Store) publishWaifLocked(image *waifTxnImage, ts uint64) {
	image.value.PublishWaifImage(s.waifDomain, ts, image.staged)
	image.base, _ = image.value.WaifImageAt(s.waifDomain, ts)
	image.staged = nil
	// Advertise history before sampling the reader floor. A last reader that
	// deregisters after its shard was sampled must see pending work and prune
	// after this publication releases store.mu, rather than miss cleanup forever.
	identity := image.value.WaifIdentity()
	lazySet(&s.waifHistory, identity, image.value.WeakWaif())
	s.waifHistoryPending.Store(true)
	if !image.value.PruneWaifImages(s.waifDomain, s.historyFloor()) {
		delete(s.waifHistory, identity)
	}
	s.waifHistoryPending.Store(len(s.waifHistory) != 0)
}

func (s *Store) pruneWaifHistoryLocked() {
	if len(s.waifHistory) == 0 {
		return
	}
	floor := s.historyFloor()
	for identity, weak := range s.waifHistory {
		value, alive := weak.Value()
		if !alive || !value.PruneWaifImages(s.waifDomain, floor) {
			delete(s.waifHistory, identity)
		}
	}
	s.waifHistoryPending.Store(len(s.waifHistory) != 0)
}

// VisitWaifValues includes private and historical values in task root capture.
func (tx *StoreTxn) VisitWaifValues(visit func(types.Value)) {
	if tx == nil || tx.direct || tx.released.Load() {
		return
	}
	for _, image := range tx.waifs {
		visit(image.value)
		image.base.VisitValues(visit)
		for _, value := range image.staged {
			visit(value)
		}
	}
}
