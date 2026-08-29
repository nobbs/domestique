package osmindex

import (
	"encoding/binary"
	"math"
	"testing"
)

// This file builds a real .osm.pbf in memory so the decode passes can be tested
// without a fixture nobody can read and without the network. The format is a
// sequence of blobs, each preceded by its header's length in network byte order,
// and a blob may hold its payload uncompressed. The field numbers below are from
// the OSM PBF schema.

type testNode struct {
	id       int64
	latitude float64

	longitude float64
}

type testWay struct {
	tags map[string]string
	refs []int64
	id   int64
}

// buildPBF encodes nodes and ways as one header blob and one data blob.
func buildPBF(nodes []testNode, ways []testWay) []byte {
	strings := newStringTable()
	group := protoBytes(2, encodeDenseNodes(nodes))
	for _, way := range ways {
		group = append(group, protoBytes(3, encodeWay(way, strings))...)
	}

	block := protoBytes(1, strings.encode())
	block = append(block, protoBytes(2, group)...)
	// Coordinates are granularity nanodegrees, so 100 makes a raw unit 1e-7 of a
	// degree — the same fixed point the builder reads them at.
	block = append(block, protoVarint(17, 100)...)

	file := blob("OSMHeader", nil)

	return append(file, blob("OSMData", block)...)
}

// blob wraps one payload as a length-prefixed header followed by the payload.
func blob(kind string, payload []byte) []byte {
	body := protoBytes(1, payload)
	header := protoBytes(1, []byte(kind))
	header = append(header, protoVarint(3, uint64(len(body)))...)

	//nolint:gosec // A header of a handful of bytes cannot overflow a uint32.
	out := binary.BigEndian.AppendUint32(nil, uint32(len(header)))
	out = append(out, header...)

	return append(out, body...)
}

// encodeDenseNodes packs every node into the one message a real extract uses:
// three parallel delta-coded columns rather than a message per node. None of
// these nodes carries a tag, so the keys_vals column is left out entirely.
func encodeDenseNodes(nodes []testNode) []byte {
	ids, latitudes, longitudes := make([]byte, 0, 32), make([]byte, 0, 32), make([]byte, 0, 32)
	var previousID, previousLatitude, previousLongitude int64
	for _, node := range nodes {
		latitude, longitude := scaled(node.latitude), scaled(node.longitude)
		ids = appendUvarint(ids, zigzag(node.id-previousID))
		latitudes = appendUvarint(latitudes, zigzag(latitude-previousLatitude))
		longitudes = appendUvarint(longitudes, zigzag(longitude-previousLongitude))
		previousID, previousLatitude, previousLongitude = node.id, latitude, longitude
	}

	out := protoBytes(1, ids)
	out = append(out, protoBytes(8, latitudes)...)

	return append(out, protoBytes(9, longitudes)...)
}

func encodeWay(way testWay, strings *stringTable) []byte {
	// A way identifier is a plain int64 in the schema, so its encoding is the
	// unsigned reading of the same bits rather than a conversion.
	out := protoVarint(1, uint64(way.id)) //nolint:gosec // Reinterpreted, not narrowed.

	keys, values := make([]byte, 0, 16), make([]byte, 0, 16)
	for key, value := range way.tags {
		keys = appendUvarint(keys, strings.id(key))
		values = appendUvarint(values, strings.id(value))
	}
	out = append(out, protoBytes(2, keys)...)
	out = append(out, protoBytes(3, values)...)

	// References are delta coded against the previous one.
	references, previous := make([]byte, 0, 16), int64(0)
	for _, reference := range way.refs {
		references = appendUvarint(references, zigzag(reference-previous))
		previous = reference
	}

	return append(out, protoBytes(8, references)...)
}

// stringTable is the per-block table tags are indexed into. Entry zero is
// reserved by the format, so the first real string is one.
type stringTable struct {
	ids     map[string]uint64
	entries []string
}

func newStringTable() *stringTable {
	return &stringTable{ids: map[string]uint64{}, entries: []string{""}}
}

func (s *stringTable) id(value string) uint64 {
	if id, found := s.ids[value]; found {
		return id
	}
	id := uint64(len(s.entries))
	s.entries = append(s.entries, value)
	s.ids[value] = id

	return id
}

func (s *stringTable) encode() []byte {
	out := make([]byte, 0, 128)
	for _, entry := range s.entries {
		out = append(out, protoBytes(1, []byte(entry))...)
	}

	return out
}

func protoVarint(field, value uint64) []byte {
	return appendUvarint(appendUvarint(nil, field<<3), value)
}

func protoBytes(field uint64, value []byte) []byte {
	out := appendUvarint(nil, field<<3|2)
	out = appendUvarint(out, uint64(len(value)))

	return append(out, value...)
}

func appendUvarint(destination []byte, value uint64) []byte {
	var buffer [binary.MaxVarintLen64]byte
	written := binary.PutUvarint(buffer[:], value)

	return append(destination, buffer[:written]...)
}

// zigzag is the signed varint encoding protobuf calls sint64.
func zigzag(value int64) uint64 {
	return uint64(value<<1) ^ uint64(value>>63) //nolint:gosec // Zigzag is defined on the bit pattern.
}

func scaled(degrees float64) int64 {
	return int64(math.Round(degrees * nativeScale))
}

// testExtract is the fixture the decode tests share: two nodes short of a
// straight line through one cell, and one way of each outcome the filter has.
func testExtract(t *testing.T) []byte {
	t.Helper()

	return buildPBF(
		[]testNode{
			{id: 1, latitude: 49.9010, longitude: 8.3010},
			{id: 2, latitude: 49.9020, longitude: 8.3020},
			{id: 3, latitude: 49.9030, longitude: 8.3030},
			// Referenced by nothing, so the node pass has to skip it.
			{id: 99, latitude: 49.9040, longitude: 8.3040},
		},
		[]testWay{
			{id: 10, refs: []int64{1, 2, 3}, tags: map[string]string{"highway": "residential", "surface": "asphalt"}},
			{id: 11, refs: []int64{1, 2}, tags: map[string]string{"highway": "track", "tracktype": "grade4"}},
			// Kept, but unsurveyed: a road nobody tagged is not a road nobody rides.
			{id: 12, refs: []int64{2, 3}, tags: map[string]string{"highway": "footway"}},
			// Each of these is filtered out for its own reason.
			{id: 13, refs: []int64{1, 2}, tags: map[string]string{"highway": "platform"}},
			{id: 14, refs: []int64{1, 2}, tags: map[string]string{"highway": "pedestrian", "area": "yes"}},
			{id: 15, refs: []int64{1, 2}, tags: map[string]string{"building": "yes"}},
		},
	)
}
