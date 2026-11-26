// Package matroska reads and rewrites the metadata identity surface of
// Matroska/WebM files using pure Go EBML.
package matroska

import "mkvidentity/ebml"

const (
	IDEBML               ebml.ID = 0x1A45DFA3
	IDSegment            ebml.ID = 0x18538067
	IDSeekHead           ebml.ID = 0x114D9B74
	IDSeek               ebml.ID = 0x4DBB
	IDSeekID             ebml.ID = 0x53AB
	IDSeekPosition       ebml.ID = 0x53AC
	IDInfo               ebml.ID = 0x1549A966
	IDTimecodeScale      ebml.ID = 0x2AD7B1
	IDDuration           ebml.ID = 0x4489
	IDTracks             ebml.ID = 0x1654AE6B
	IDTrackEntry         ebml.ID = 0xAE
	IDTrackNumber        ebml.ID = 0xD7
	IDTrackUID           ebml.ID = 0x73C5
	IDTrackType          ebml.ID = 0x83
	IDFlagEnabled        ebml.ID = 0xB9
	IDFlagDefault        ebml.ID = 0x88
	IDFlagForced         ebml.ID = 0x55AA
	IDName               ebml.ID = 0x536E
	IDLanguage           ebml.ID = 0x22B59C
	IDLanguageIETF       ebml.ID = 0x22B59D
	IDCodecID            ebml.ID = 0x86
	IDCodecPrivate       ebml.ID = 0x63A2
	IDAudio              ebml.ID = 0xE1
	IDChannels           ebml.ID = 0x9F
	IDSamplingFrequency  ebml.ID = 0xB5
	IDChapters           ebml.ID = 0x1043A770
	IDEditionEntry       ebml.ID = 0x45B9
	IDEditionUID         ebml.ID = 0x45BC
	IDEditionDefault     ebml.ID = 0x45DB
	IDEditionOrdered     ebml.ID = 0x45DD
	IDChapterAtom        ebml.ID = 0xB6
	IDChapterUID         ebml.ID = 0x73C4
	IDChapterStringUID   ebml.ID = 0x5654
	IDChapterTimeStart   ebml.ID = 0x91
	IDChapterTimeEnd     ebml.ID = 0x92
	IDChapterEnabled     ebml.ID = 0x4598
	IDChapterTrack       ebml.ID = 0x8F
	IDChapterTrackUID    ebml.ID = 0x89
	IDChapterDisplay     ebml.ID = 0x80
	IDChapString         ebml.ID = 0x85
	IDChapLanguage       ebml.ID = 0x437C
	IDChapLanguageIETF   ebml.ID = 0x437D
	IDCues               ebml.ID = 0x1C53BB6B
	IDCuePoint           ebml.ID = 0xBB
	IDCueTrackPositions  ebml.ID = 0xB7
	IDCueClusterPosition ebml.ID = 0xF1
)

var Containers = map[ebml.ID]bool{
	IDEBML: true, IDSegment: true, IDSeekHead: true, IDSeek: true,
	IDInfo: true, IDTracks: true, IDTrackEntry: true,
	IDAudio:    true,
	IDChapters: true, IDEditionEntry: true, IDChapterAtom: true,
	IDChapterTrack: true, IDChapterDisplay: true,
	IDCues: true, IDCuePoint: true, IDCueTrackPositions: true,
}
