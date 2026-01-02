package storage

import "github.com/klauspost/compress/zstd"

var compressEncoder, _ = zstd.NewWriter(nil)

func CompressBytes(src []byte) []byte {
	return compressEncoder.EncodeAll(src, make([]byte, 0, len(src)))
}

// Create a reader that caches decompressors.
// For this operation type we supply a nil Reader.
var compressdecoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))

// Decompress a buffer. We don't supply a destination buffer,
// so it will be allocated by the decoder.
func DecompressBytes(src []byte) ([]byte, error) {
	return compressdecoder.DecodeAll(src, nil)
}
