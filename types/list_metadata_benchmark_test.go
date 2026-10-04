package types

import (
	"strconv"
	"testing"
	"unsafe"
)

var listMetadataValueSink Value
var listMetadataSizeSink int
var listMetadataFinalizableSink bool

func BenchmarkListMetadata(b *testing.B) {
	for _, length := range []int{8, 1024} {
		b.Run("Constructor"+strconv.Itoa(length), func(b *testing.B) {
			elements := make([]Value, length)
			for i := range elements {
				elements[i] = NewInt(int64(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				listMetadataValueSink = NewList(elements)
			}
			b.StopTimer()
			b.ReportMetric(float64(unsafe.Sizeof(sliceList{})), "header-B")
			if listMetadataValueSink.Len() != length || ValueBytes(listMetadataValueSink) != listVarOverhead+length*valueVarSize || listMetadataValueSink.MayHoldFinalizable() {
				b.Fatal("constructor workload changed")
			}
		})
	}
	for _, length := range []int{32, 1024} {
		b.Run("Append"+strconv.Itoa(length), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				list := NewEmptyList()
				for value := 0; value < length; value++ {
					list = list.Append(NewInt(int64(value)))
				}
				listMetadataValueSink = list
			}
			b.StopTimer()
			if listMetadataValueSink.Len() != length || listMetadataValueSink.Get(length).Int() != int64(length-1) || ValueBytes(listMetadataValueSink) != listVarOverhead+length*valueVarSize || listMetadataValueSink.MayHoldFinalizable() {
				b.Fatal("append workload changed")
			}
		})
	}
	b.Run("WarmReads", func(b *testing.B) {
		shared := coldListMetadataFixture(true)
		ValueBytes(shared)
		shared.MayHoldFinalizable()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			listMetadataSizeSink = ValueBytes(shared)
			listMetadataFinalizableSink = shared.MayHoldFinalizable()
		}
		b.StopTimer()
		if listMetadataSizeSink != 208 || !listMetadataFinalizableSink {
			b.Fatal("warm metadata workload changed")
		}
	})
	b.Run("ColdReads", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			shared := coldListMetadataFixture(true)
			listMetadataSizeSink = ValueBytes(shared)
			listMetadataFinalizableSink = shared.MayHoldFinalizable()
			listMetadataValueSink = shared
		}
		b.StopTimer()
		if listMetadataSizeSink != 208 || !listMetadataFinalizableSink {
			b.Fatal("cold metadata workload changed")
		}
	})
}
