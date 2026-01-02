package storage

import (
	"bytes"
	"testing"
)

func TestCompressAndDecompressBytes(t *testing.T) {
	original := []byte("The quick brown fox jumps over the lazy dog")
	compressed := CompressBytes(original)
	if bytes.Equal(original, compressed) {
		t.Error("Compressed data should differ from original")
	}

	decompressed, err := DecompressBytes(compressed)
	if err != nil {
		t.Fatalf("DecompressBytes returned error: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Errorf("Decompressed data does not match original.\nGot:  %v\nWant: %v", decompressed, original)
	}
}

func TestCompressBytes_EmptyInput(t *testing.T) {
	original := []byte{}
	compressed := CompressBytes(original)
	decompressed, err := DecompressBytes(compressed)
	if err != nil {
		t.Fatalf("DecompressBytes returned error: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Errorf("Decompressed empty input does not match original.\nGot:  %v\nWant: %v", decompressed, original)
	}
}

func TestDecompressBytes_InvalidInput(t *testing.T) {
	invalid := []byte("not a valid zstd stream")
	_, err := DecompressBytes(invalid)
	if err == nil {
		t.Error("Expected error when decompressing invalid input, got nil")
	}
}
