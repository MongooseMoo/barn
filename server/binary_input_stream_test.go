package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
)

func TestTCPReadChunkPreservesPendingText(t *testing.T) {
	for _, test := range []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "ASCII", prefix: "audi", want: "auditbin"},
		{name: "literal_tilde", prefix: "~", want: "~tbin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(test.prefix), tnIAC, tnWILL, 1)
			data = append(data, "tbin"...)
			transport := newTestTransport(data)
			line, oob, err := transport.ReadInput()
			if err != nil || !oob || line != "~FF~FB~01" {
				t.Fatalf("ReadInput = %q, %v, %v; want OOB negotiation", line, oob, err)
			}
			if got := transport.lineBuf.String(); got != test.prefix {
				t.Fatalf("pending prefix = %q, want %q", got, test.prefix)
			}
			got, err := transport.ReadChunk()
			if err != nil || got != test.want {
				t.Fatalf("ReadChunk = %q, %v; want %q", got, err, test.want)
			}
			if transport.lineBuf.Len() != 0 {
				t.Fatal("successful binary dispatch retained its pending input")
			}
		})
	}
}

func TestTCPReadChunkEncodesExternalBytes(t *testing.T) {
	transport := newTestTransport([]byte{'~', 0, '\r', '\n', 255})
	got, err := transport.ReadChunk()
	if want := "~7E~00~0D~0A~FF"; err != nil || got != want {
		t.Fatalf("ReadChunk = %q, %v; want %q", got, err, want)
	}
}

func TestTCPReadChunkRetainsPrefixOnInitialError(t *testing.T) {
	data := append([]byte("pending"), tnIAC, tnWILL, 1)
	transport := newTestTransport(data)
	line, oob, err := transport.ReadInput()
	if err != nil || !oob || line != "~FF~FB~01" {
		t.Fatalf("ReadInput = %q, %v, %v; want OOB negotiation", line, oob, err)
	}
	got, err := transport.ReadChunk()
	if got != "" || !errors.Is(err, io.EOF) {
		t.Fatalf("empty binary read = %q, %v; want EOF", got, err)
	}
	line, oob, err = transport.ReadInput()
	if err != nil || oob || line != "pending" {
		t.Fatalf("retained text read = %q, %v, %v; want pending", line, oob, err)
	}
}

func TestTCPReadChunkIndependentReadBoundaries(t *testing.T) {
	data := append([]byte("~"), tnIAC, tnWILL, 1)
	data = append(data, '~', 0, '\r', '\n', 255)
	transport := NewTCPTransport(&tcpFuzzConn{
		fakeConn: &fakeConn{bytes.NewReader(data)},
		chunks:   []byte{0},
	})
	line, oob, err := transport.ReadInput()
	if err != nil || !oob || line != "~FF~FB~01" {
		t.Fatalf("ReadInput = %q, %v, %v; want OOB negotiation", line, oob, err)
	}
	var got strings.Builder
	for range len(data) + 1 {
		chunk, err := transport.ReadChunk()
		if errors.Is(err, io.EOF) {
			if want := "~~7E~00~0D~0A~FF"; got.String() != want {
				t.Fatalf("concatenated chunks = %q, want %q", got.String(), want)
			}
			return
		}
		if err != nil {
			t.Fatalf("ReadChunk failed: %v", err)
		}
		got.WriteString(chunk)
	}
	t.Fatal("binary reads did not terminate within the input byte budget")
}

func TestRawToBinaryLiteralOracle(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	want := "~00~01~02~03~04~05~06~07~08~09~0A~0B~0C~0D~0E~0F" +
		"~10~11~12~13~14~15~16~17~18~19~1A~1B~1C~1D~1E~1F" +
		" !\"#$%&'()*+,-./" +
		"0123456789:;<=>?" +
		"@ABCDEFGHIJKLMNO" +
		"PQRSTUVWXYZ[\\]^_" +
		"`abcdefghijklmno" +
		"pqrstuvwxyz{|}~7E~7F" +
		"~80~81~82~83~84~85~86~87~88~89~8A~8B~8C~8D~8E~8F" +
		"~90~91~92~93~94~95~96~97~98~99~9A~9B~9C~9D~9E~9F" +
		"~A0~A1~A2~A3~A4~A5~A6~A7~A8~A9~AA~AB~AC~AD~AE~AF" +
		"~B0~B1~B2~B3~B4~B5~B6~B7~B8~B9~BA~BB~BC~BD~BE~BF" +
		"~C0~C1~C2~C3~C4~C5~C6~C7~C8~C9~CA~CB~CC~CD~CE~CF" +
		"~D0~D1~D2~D3~D4~D5~D6~D7~D8~D9~DA~DB~DC~DD~DE~DF" +
		"~E0~E1~E2~E3~E4~E5~E6~E7~E8~E9~EA~EB~EC~ED~EE~EF" +
		"~F0~F1~F2~F3~F4~F5~F6~F7~F8~F9~FA~FB~FC~FD~FE~FF"
	if got := builtins.EncodeRawToBinary(data); got != want {
		t.Fatalf("all-byte encoding = %q; want %q", got, want)
	}
	for _, test := range []struct {
		raw  []byte
		want string
	}{
		{raw: nil, want: ""},
		{raw: []byte("ordinary ASCII"), want: "ordinary ASCII"},
		{raw: []byte{'a', 0, 'b', '~', 255, 'c'}, want: "a~00b~7E~FFc"},
	} {
		if got := builtins.EncodeRawToBinary(test.raw); got != test.want {
			t.Fatalf("encoding %q = %q; want %q", test.raw, got, test.want)
		}
	}
}

func TestAppendRawToBinaryPreservesPrefix(t *testing.T) {
	var result strings.Builder
	result.WriteString("text~prefix")
	builtins.AppendRawToBinary(&result, []byte{'a', 0, 'b', '~', 255, 'c'})
	if got, want := result.String(), "text~prefixa~00b~7E~FFc"; got != want {
		t.Fatalf("appended binary = %q; want %q", got, want)
	}
}

func BenchmarkRawToBinary(b *testing.B) {
	for _, size := range []int{1, 1024, 4096} {
		for _, shape := range []struct {
			name    string
			raw     []byte
			encoded []string
		}{
			{name: "ASCII", raw: []byte{'a'}, encoded: []string{"a"}},
			{name: "special", raw: []byte{'~', 0, '\r', '\n', 255}, encoded: []string{"~7E", "~00", "~0D", "~0A", "~FF"}},
			{name: "mixed", raw: []byte{'a', 'b', 0, 'c', '~', 255, 'd', '\n'}, encoded: []string{"a", "b", "~00", "c", "~7E", "~FF", "d", "~0A"}},
		} {
			b.Run(fmt.Sprintf("%s/bytes=%d", shape.name, size), func(b *testing.B) {
				data := make([]byte, size)
				var expected strings.Builder
				for i := range data {
					data[i] = shape.raw[i%len(shape.raw)]
					expected.WriteString(shape.encoded[i%len(shape.encoded)])
				}
				want := expected.String()
				b.SetBytes(int64(size))
				b.ReportAllocs()
				for b.Loop() {
					if got := builtins.EncodeRawToBinary(data); got != want {
						b.Fatalf("EncodeRawToBinary = %q; want %q", got, want)
					}
				}
			})
		}
	}
}

func BenchmarkTCPReadChunkASCII(b *testing.B) {
	for _, size := range []int{1, 1024, 4096} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			data := bytes.Repeat([]byte{'a'}, size)
			want := string(data)
			reader := bytes.NewReader(data)
			conn := &fakeConn{reader}
			transport := NewTCPTransport(conn)
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				reader.Reset(data)
				transport.reader.Reset(conn)
				got, err := transport.ReadChunk()
				if err != nil || got != want {
					b.Fatalf("ReadChunk returned %q, %v; want %d ASCII bytes", got, err, size)
				}
			}
		})
	}
}
