package ebml

// Matroska/WebM element identifiers used by alphacheck.
const (
	IDEbml              ID = 0x1A45DFA3
	IDSegment           ID = 0x18538067
	IDSeekHead          ID = 0x114D9B74
	IDSeek              ID = 0x4DBB
	IDInfo              ID = 0x1549A966
	IDTimecodeScale     ID = 0x2AD7B1
	IDMuxingApp         ID = 0x4D80
	IDWritingApp        ID = 0x5741
	IDDuration          ID = 0x4489
	IDTracks            ID = 0x1654AE6B
	IDTrackEntry        ID = 0xAE
	IDTrackNumber       ID = 0xD7
	IDTrackUID          ID = 0x73C5
	IDTrackType         ID = 0x83
	IDCodecID           ID = 0x86
	IDCodecPrivate      ID = 0x63A2
	IDDefaultDuration   ID = 0x23E383
	IDVideo             ID = 0xE0
	IDPixelWidth        ID = 0xB0
	IDPixelHeight       ID = 0xBA
	IDDisplayWidth      ID = 0x54B0
	IDDisplayHeight     ID = 0x54BA
	IDDisplayUnit       ID = 0x54B2
	IDAlphaMode         ID = 0x53C0
	IDCropBottom        ID = 0x54AA
	IDCropTop           ID = 0x54BB
	IDCropLeft          ID = 0x54CC
	IDCropRight         ID = 0x54DD
	IDCluster           ID = 0x1F43B675
	IDTimecode          ID = 0xE7
	IDSimpleBlock       ID = 0xA3
	IDBlockGroup        ID = 0xA0
	IDBlock             ID = 0xA1
	IDBlockDuration     ID = 0x9B
	IDReferenceBlock    ID = 0xFB
	IDBlockAdditions    ID = 0x75A1
	IDBlockMore         ID = 0xA6
	IDBlockAddID        ID = 0xEE
	IDBlockAdditional   ID = 0xA5
	IDCues              ID = 0x1C53BB6B
	IDCuePoint          ID = 0xBB
	IDCueTrackPositions ID = 0xB7
)

var containers = map[ID]bool{
	IDEbml:              true,
	IDSegment:           true,
	IDSeekHead:          true,
	IDSeek:              true,
	IDInfo:              true,
	IDTracks:            true,
	IDTrackEntry:        true,
	IDVideo:             true,
	IDCluster:           true,
	IDBlockGroup:        true,
	IDBlockAdditions:    true,
	IDBlockMore:         true,
	IDCues:              true,
	IDCuePoint:          true,
	IDCueTrackPositions: true,
}
