package types

import (
	"maps"
	"unsafe"
	"weak"
)

// WaifDomain identifies one publication clock. Its address, not its contents,
// is the identity; the byte prevents zero-size pointer coalescing.
type WaifDomain struct{ identity byte }

// WaifImage is an immutable property image owned by one publication clock.
type WaifImage struct {
	timestamp  uint64
	properties map[string]Value
}

func (i *WaifImage) Timestamp() uint64                  { return i.timestamp }
func (i *WaifImage) Property(name string) (Value, bool) { v, ok := i.properties[name]; return v, ok }
func (i *WaifImage) Properties() map[string]Value       { return maps.Clone(i.properties) }
func (i *WaifImage) VisitValues(visit func(Value)) {
	for _, value := range i.properties {
		visit(value)
	}
}

// WaifImageAt attaches a detached value to a clock and selects its snapshot.
// Detached constructor state predates every publication on that clock.
func (v Value) WaifImageAt(domain *WaifDomain, timestamp uint64) (*WaifImage, bool) {
	w := v.waifRep()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.domain == nil {
		w.domain = domain
		w.images = []*WaifImage{{properties: maps.Clone(w.properties)}}
	} else if w.domain != domain {
		return nil, false
	}
	for n := len(w.images) - 1; n >= 0; n-- {
		if w.images[n].timestamp <= timestamp {
			return w.images[n], true
		}
	}
	return nil, false
}

// PublishWaifImage is called under the owning store's publication lock after
// validation. Copying the map prevents a transaction retaining a mutable alias.
func (v Value) PublishWaifImage(domain *WaifDomain, timestamp uint64, properties map[string]Value) {
	w := v.waifRep()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.domain != domain || len(w.images) == 0 || timestamp <= w.images[len(w.images)-1].timestamp {
		panic("invalid WAIF publication domain or timestamp")
	}
	image := &WaifImage{timestamp: timestamp, properties: maps.Clone(properties)}
	referencesChanged := !waifImageReferencesEqual(w.images[len(w.images)-1], image)
	w.images = append(w.images, image)
	w.properties = image.properties
	if referencesChanged {
		waifGraphEpoch.Add(1)
	}
}

// PruneWaifImages retains the newest image at or below the live reader floor
// and all later images. It reports whether historical images remain.
func (v Value) PruneWaifImages(domain *WaifDomain, floor uint64) bool {
	w := v.waifRep()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.domain != domain {
		panic("foreign WAIF history domain")
	}
	keep := 0
	for n := len(w.images) - 1; n >= 0; n-- {
		if w.images[n].timestamp <= floor {
			keep = n
			break
		}
	}
	if keep > 0 {
		for _, image := range w.images[:keep] {
			if !waifImageReferencesEqual(image, w.images[keep]) {
				waifGraphEpoch.Add(1)
				break
			}
		}
		w.images = append([]*WaifImage(nil), w.images[keep:]...)
	}
	return len(w.images) > 1
}

func waifImageReferencesEqual(a, b *WaifImage) bool {
	for name, value := range a.properties {
		if value.MayHoldFinalizable() && b.properties[name] != value {
			return false
		}
	}
	for name, value := range b.properties {
		if value.MayHoldFinalizable() && a.properties[name] != value {
			return false
		}
	}
	return true
}

// VisitRetainedWaifValues is for semantic garbage collection only. An old
// snapshot can first dereference a WAIF after another task replaces its current
// properties, so every still-readable image contributes liveness. Callbacks run
// without the payload lock and may recursively visit other WAIFs.
func (v Value) VisitRetainedWaifValues(visit func(Value)) {
	w := v.waifRep()
	w.mu.RLock()
	images := append([]*WaifImage(nil), w.images...)
	var detached map[string]Value
	if len(images) == 0 {
		detached = maps.Clone(w.properties)
	}
	w.mu.RUnlock()
	for _, image := range images {
		image.VisitValues(visit)
	}
	for _, value := range detached {
		visit(value)
	}
}

// WeakWaif allows history housekeeping without turning the store into a root.
type WeakWaif struct{ pointer weak.Pointer[waifRep] }

func (v Value) WeakWaif() WeakWaif { return WeakWaif{weak.Make(v.waifRep())} }
func (w WeakWaif) Value() (Value, bool) {
	p := w.pointer.Value()
	if p == nil {
		return Value{}, false
	}
	return Value{tag: TYPE_WAIF, ref: unsafe.Pointer(p)}, true
}
