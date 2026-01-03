package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"regexp"
	"strings"
	"unicode/utf8"

	"waddlemap/internal/types"
)

const (
	// CurrentHeaderSize is the current version's header size in bytes.
	CurrentHeaderSize = 18

	// MaxKeyLength is the maximum key length in bytes (65KB).
	MaxKeyLength = 65535

	// MaxKeywordLength is the maximum keyword length in bytes.
	MaxKeywordLength = 128

	// MaxKeywordsBlockSize is the maximum size of keywords block (65KB).
	MaxKeywordsBlockSize = 65535
)

// Entry represents a complete database entry with vector store support.
type Entry struct {
	Flags         types.EntryFlags
	Key           []byte
	Keywords      []string
	PrimaryData   []byte
	SecondaryData []byte // VectorID bytes for vector entries
}

// EntryHeader represents the on-disk entry header (18 bytes minimum).
type EntryHeader struct {
	HeaderSize   uint8  // Byte 0: Total header size (currently 18)
	Flags        uint8  // Byte 1: Bitmask for data types and state
	KeyLen       uint16 // Bytes 2-3: Length of key
	PrimaryLen   uint32 // Bytes 4-7: Length of primary data
	SecondaryLen uint32 // Bytes 8-11: Length of secondary data
	KwLen        uint16 // Bytes 12-13: Length of serialized keywords block
	CRC32        uint32 // Bytes 14-17: Checksum of entire entry
}

// keywordRegex validates keyword characters (a-z, 0-9, _, -).
var keywordRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)

// ValidateKeyword checks if a keyword meets the specification requirements.
func ValidateKeyword(keyword string) error {
	if len(keyword) == 0 {
		return errors.New("keyword cannot be empty")
	}
	if len(keyword) > MaxKeywordLength {
		return fmt.Errorf("keyword exceeds maximum length of %d bytes", MaxKeywordLength)
	}
	if !utf8.ValidString(keyword) {
		return errors.New("keyword must be valid UTF-8")
	}
	normalized := strings.ToLower(keyword)
	if !keywordRegex.MatchString(normalized) {
		return errors.New("keyword may only contain a-z, 0-9, underscore, and dash")
	}
	return nil
}

// NormalizeKeyword converts a keyword to lowercase for storage.
func NormalizeKeyword(keyword string) string {
	return strings.ToLower(keyword)
}

// EncodeKeywords serializes keywords into the binary format.
// Format: [Count (2B)] [Len1 (1B)][Keyword1 Bytes] [Len2 (1B)][Keyword2 Bytes] ...
func EncodeKeywords(keywords []string) ([]byte, error) {
	if len(keywords) > 65535 {
		return nil, errors.New("too many keywords (max 65535)")
	}

	buf := new(bytes.Buffer)

	// Write count (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, uint16(len(keywords))); err != nil {
		return nil, err
	}

	// Write each keyword
	for _, kw := range keywords {
		normalized := NormalizeKeyword(kw)
		if err := ValidateKeyword(normalized); err != nil {
			return nil, fmt.Errorf("invalid keyword %q: %w", kw, err)
		}
		kwBytes := []byte(normalized)
		if len(kwBytes) > 255 {
			return nil, fmt.Errorf("keyword %q exceeds 255 bytes", kw)
		}
		// Write length (1 byte)
		buf.WriteByte(uint8(len(kwBytes)))
		// Write keyword bytes
		buf.Write(kwBytes)
	}

	if buf.Len() > MaxKeywordsBlockSize {
		return nil, fmt.Errorf("keywords block exceeds maximum size of %d bytes", MaxKeywordsBlockSize)
	}

	return buf.Bytes(), nil
}

// DecodeKeywords deserializes keywords from the binary format.
func DecodeKeywords(data []byte) ([]string, error) {
	if len(data) < 2 {
		return nil, errors.New("keywords data too short")
	}

	buf := bytes.NewReader(data)

	var count uint16
	if err := binary.Read(buf, binary.BigEndian, &count); err != nil {
		return nil, err
	}

	keywords := make([]string, 0, count)
	for i := uint16(0); i < count; i++ {
		lenByte, err := buf.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read keyword length at index %d: %w", i, err)
		}
		kwBytes := make([]byte, lenByte)
		if _, err := buf.Read(kwBytes); err != nil {
			return nil, fmt.Errorf("failed to read keyword at index %d: %w", i, err)
		}
		keywords = append(keywords, string(kwBytes))
	}

	return keywords, nil
}

// EncodeEntry serializes an Entry to the on-disk binary format.
func EncodeEntry(entry *Entry) ([]byte, error) {
	// Encode keywords
	kwBytes, err := EncodeKeywords(entry.Keywords)
	if err != nil {
		return nil, fmt.Errorf("failed to encode keywords: %w", err)
	}

	// Validate lengths
	if len(entry.Key) > MaxKeyLength {
		return nil, fmt.Errorf("key exceeds maximum length of %d bytes", MaxKeyLength)
	}

	// Build header
	header := EntryHeader{
		HeaderSize:   CurrentHeaderSize,
		Flags:        types.EncodeFlags(entry.Flags),
		KeyLen:       uint16(len(entry.Key)),
		PrimaryLen:   uint32(len(entry.PrimaryData)),
		SecondaryLen: uint32(len(entry.SecondaryData)),
		KwLen:        uint16(len(kwBytes)),
		CRC32:        0, // Will be calculated after
	}

	// Calculate total size
	totalSize := int(CurrentHeaderSize) + len(entry.Key) + len(kwBytes) +
		len(entry.PrimaryData) + len(entry.SecondaryData)
	buf := make([]byte, 0, totalSize)
	bufWriter := bytes.NewBuffer(buf)

	// Write header (without CRC)
	bufWriter.WriteByte(header.HeaderSize)
	bufWriter.WriteByte(header.Flags)
	binary.Write(bufWriter, binary.BigEndian, header.KeyLen)
	binary.Write(bufWriter, binary.BigEndian, header.PrimaryLen)
	binary.Write(bufWriter, binary.BigEndian, header.SecondaryLen)
	binary.Write(bufWriter, binary.BigEndian, header.KwLen)
	binary.Write(bufWriter, binary.BigEndian, header.CRC32) // placeholder

	// Write data
	bufWriter.Write(entry.Key)
	bufWriter.Write(kwBytes)
	bufWriter.Write(entry.PrimaryData)
	bufWriter.Write(entry.SecondaryData)

	// Calculate and set CRC32
	result := bufWriter.Bytes()
	checksum := crc32.ChecksumIEEE(result)
	binary.BigEndian.PutUint32(result[14:18], checksum)

	return result, nil
}

// DecodeEntryHeader reads and parses the entry header from data.
func DecodeEntryHeader(data []byte) (*EntryHeader, error) {
	if len(data) < CurrentHeaderSize {
		return nil, errors.New("data too short for header")
	}

	headerSize := data[0]
	if int(headerSize) > len(data) {
		return nil, errors.New("header size exceeds data length")
	}

	header := &EntryHeader{
		HeaderSize:   headerSize,
		Flags:        data[1],
		KeyLen:       binary.BigEndian.Uint16(data[2:4]),
		PrimaryLen:   binary.BigEndian.Uint32(data[4:8]),
		SecondaryLen: binary.BigEndian.Uint32(data[8:12]),
		KwLen:        binary.BigEndian.Uint16(data[12:14]),
		CRC32:        binary.BigEndian.Uint32(data[14:18]),
	}

	return header, nil
}

// DecodeEntry deserializes an Entry from the on-disk binary format.
func DecodeEntry(data []byte) (*Entry, error) {
	header, err := DecodeEntryHeader(data)
	if err != nil {
		return nil, err
	}

	// Validate CRC
	storedCRC := header.CRC32
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	binary.BigEndian.PutUint32(dataCopy[14:18], 0) // Zero out CRC for calculation
	calculatedCRC := crc32.ChecksumIEEE(dataCopy)
	if storedCRC != calculatedCRC {
		return nil, fmt.Errorf("CRC mismatch: stored=%08x calculated=%08x", storedCRC, calculatedCRC)
	}

	// Calculate offsets
	keyStart := int(header.HeaderSize)
	keyEnd := keyStart + int(header.KeyLen)
	kwEnd := keyEnd + int(header.KwLen)
	primaryEnd := kwEnd + int(header.PrimaryLen)
	secondaryEnd := primaryEnd + int(header.SecondaryLen)

	if secondaryEnd > len(data) {
		return nil, errors.New("data truncated")
	}

	// Extract data sections
	key := data[keyStart:keyEnd]
	kwData := data[keyEnd:kwEnd]
	primaryData := data[kwEnd:primaryEnd]
	secondaryData := data[primaryEnd:secondaryEnd]

	// Decode keywords
	keywords, err := DecodeKeywords(kwData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode keywords: %w", err)
	}

	return &Entry{
		Flags:         types.ParseFlags(header.Flags),
		Key:           key,
		Keywords:      keywords,
		PrimaryData:   primaryData,
		SecondaryData: secondaryData,
	}, nil
}

// CalculateTotalSize returns the total size of an entry in bytes.
func CalculateTotalSize(entry *Entry) (int, error) {
	kwBytes, err := EncodeKeywords(entry.Keywords)
	if err != nil {
		return 0, err
	}
	return CurrentHeaderSize + len(entry.Key) + len(kwBytes) +
		len(entry.PrimaryData) + len(entry.SecondaryData), nil
}
