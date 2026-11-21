package builtins

// sqliteHandleOrder reproduces the iteration order of Toast's
// std::unordered_map<int, sqlite_conn*> sqlite_connections (sqlite.cc:22),
// which sqlite_handles() walks (sqlite.cc:325-326). Toast is built with GCC's
// libstdc++, whose hashtable keeps every element on one singly linked list:
//
//   - std::hash<int> is the identity, so a handle's bucket is handle % buckets;
//   - inserting into an empty bucket puts the node at the front of the list,
//     and into a non-empty bucket at the start of that bucket's run;
//   - erasing unlinks the node and never shrinks the bucket count;
//   - before an insert that would exceed the load factor (1.0) the table
//     rehashes: it walks the old list in order and places each node the same
//     way, which reverses nodes that land in distinct buckets.
//
// The bucket count only grows over the server's life, so the order depends on
// every handle opened since start-up. On stock Toast 2.8.0_7 fifteen handles
// opened on a fresh server list as {15, 14, 1, 2, ..., 13}: 13 buckets on the
// first insert, 29 from the fourteenth.
type sqliteHandleOrder struct {
	list       []int64 // iteration order
	buckets    uint64
	nextResize uint64
}

// libstdc++ _Prime_rehash_policy::_M_next_bkt tables (hashtable_c++0x.cc,
// hashtable-aux.cc): a fast table below 14, then the first listed prime not
// below n. Bucket counts reached by doubling from 13 are 29, 59, 127, 257, 541
// and 1109, all in this list; larger tables would need 1109 open handles.
var (
	sqliteFastBuckets = [...]uint64{2, 2, 2, 3, 5, 5, 7, 7, 11, 11, 11, 11, 13, 13}
	sqlitePrimeList   = [...]uint64{
		2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59, 61, 67, 71,
		73, 79, 83, 89, 97, 103, 109, 113, 127, 137, 139, 149, 157, 167, 179, 193,
		199, 211, 227, 241, 257, 277, 293, 313, 337, 359, 383, 409, 439, 467, 503,
		541, 577, 619, 661, 709, 761, 823, 887, 953, 1031, 1109,
	}
)

func (o *sqliteHandleOrder) nextBucketCount(n uint64) uint64 {
	if n < uint64(len(sqliteFastBuckets)) {
		if n == 0 {
			return 1
		}
		o.nextResize = sqliteFastBuckets[n]
		return sqliteFastBuckets[n]
	}
	for _, p := range sqlitePrimeList[6:] {
		if p >= n {
			o.nextResize = p
			return p
		}
	}
	o.nextResize = n
	return n
}

func (o *sqliteHandleOrder) bucket(handle int64) uint64 {
	return uint64(handle) % o.buckets
}

// place links handle into the list as libstdc++ _M_insert_bucket_begin does.
func (o *sqliteHandleOrder) place(handle int64) {
	b := o.bucket(handle)
	for i, other := range o.list {
		if o.bucket(other) == b {
			o.list = append(o.list[:i], append([]int64{handle}, o.list[i:]...)...)
			return
		}
	}
	o.list = append([]int64{handle}, o.list...)
}

func (o *sqliteHandleOrder) insert(handle int64) {
	if o.buckets == 0 {
		o.buckets = 1
	}
	// _M_need_rehash(bucket_count, element_count, 1) with max_load_factor 1.0.
	elements := uint64(len(o.list))
	if elements+1 > o.nextResize {
		minBuckets := elements + 1
		if o.nextResize == 0 && minBuckets < 11 {
			minBuckets = 11
		}
		if minBuckets >= o.buckets {
			old := o.list
			o.buckets = o.nextBucketCount(max(minBuckets+1, o.buckets*2))
			o.list = make([]int64, 0, len(old)+1)
			for _, h := range old {
				o.place(h)
			}
		} else {
			o.nextResize = o.buckets
		}
	}
	o.place(handle)
}

func (o *sqliteHandleOrder) erase(handle int64) {
	for i, h := range o.list {
		if h == handle {
			o.list = append(o.list[:i], o.list[i+1:]...)
			return
		}
	}
}
