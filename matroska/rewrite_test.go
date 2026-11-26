package matroska

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"mkvidentity/ebml"
	"mkvidentity/identity"
	"mkvidentity/timepoint"
)

const testClusterID ebml.ID = 0x1F43B675

func TestRewriteTracksChaptersAndIndexOffsets(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "old.mkv")
	if err := os.WriteFile(sourcePath, buildTestMKV(t), 0o644); err != nil {
		t.Fatal(err)
	}

	inv, err := ReadInventoryPath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Tracks) != 2 || inv.Tracks[0].UID != 101 {
		t.Fatalf("tracks = %+v", inv.Tracks)
	}
	if len(inv.Chapters) != 1 || len(inv.Chapters[0].TrackUIDs) != 1 || inv.Chapters[0].TrackUIDs[0] != 101 {
		t.Fatalf("chapters = %+v", inv.Chapters)
	}

	patch := identity.Patch{
		TrackPatches: []identity.TrackPatch{
			{TargetUID: 101, Default: false, SetDefault: true},
			{TargetUID: 102, Language: "chi", SetLanguage: true, Default: true, SetDefault: true},
		},
		ReplaceChapters: true,
		Chapters: []identity.Chapter{{
			UID: 700, EditionUID: 500, Ordinal: 1,
			Start:     timepoint.Resolved(1_000_000_000),
			TrackUIDs: []uint64{102},
			Displays:  []identity.ChapterDisplay{{String: "Reel A"}},
			Enabled:   true,
		}},
	}
	outPath := filepath.Join(t.TempDir(), "new.mkv")
	result, err := RewriteFile(sourcePath, outPath, patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MissingTrackUIDs) != 0 || result.SeekEntriesFixed == 0 || result.CuePositionsFixed != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	outInv, err := ReadInventoryPath(outPath)
	if err != nil {
		t.Fatal(err)
	}
	byUID := map[uint64]identity.Track{}
	for _, track := range outInv.Tracks {
		byUID[track.UID] = track
	}
	if byUID[101].Default {
		t.Fatal("track 101 default flag was not cleared")
	}
	if got := byUID[102].Language; got != "chi" {
		t.Fatalf("track 102 language = %q, want chi", got)
	}
	if !byUID[102].Default {
		t.Fatal("track 102 default flag was not set")
	}
	if len(outInv.Chapters) != 1 || outInv.Chapters[0].TrackUIDs[0] != 102 {
		t.Fatalf("rewritten chapter refs = %+v", outInv.Chapters)
	}
	assertIndexTargets(t, outPath)
}

func buildTestMKV(t *testing.T) []byte {
	t.Helper()
	track1 := ebml.NewContainer(IDTrackEntry,
		ebml.NewUintNode(IDTrackNumber, 1), ebml.NewUintNode(IDTrackUID, 101),
		ebml.NewUintNode(IDTrackType, uint64(identity.TrackAudio)),
		ebml.NewStringNode(IDLanguage, "eng"), ebml.NewUintNode(IDFlagDefault, 1),
	)
	track2 := ebml.NewContainer(IDTrackEntry,
		ebml.NewUintNode(IDTrackNumber, 2), ebml.NewUintNode(IDTrackUID, 102),
		ebml.NewUintNode(IDTrackType, uint64(identity.TrackAudio)),
		ebml.NewStringNode(IDLanguage, "jpn"), ebml.NewUintNode(IDFlagDefault, 0),
	)
	tracks := ebml.NewContainer(IDTracks, track1, track2)
	info := ebml.NewContainer(IDInfo, ebml.NewUintNode(IDTimecodeScale, 1_000_000))

	chapterTrack := ebml.NewContainer(IDChapterTrack, ebml.NewUintNode(IDChapterTrackUID, 101))
	display := ebml.NewContainer(IDChapterDisplay, ebml.NewStringNode(IDChapString, "Old Start"))
	atom := ebml.NewContainer(IDChapterAtom,
		ebml.NewUintNode(IDChapterUID, 700),
		ebml.NewUintNode(IDChapterTimeStart, 0),
		chapterTrack, display,
	)
	edition := ebml.NewContainer(IDEditionEntry, ebml.NewUintNode(IDEditionUID, 500), atom)
	chapters := ebml.NewContainer(IDChapters, edition)
	cluster := ebml.ElementBytes(testClusterID, []byte{0x00, 0x01, 0x02, 0x03})

	seekHead := buildSeekHeadWithFixedPositions()
	seekBytes := seekHead.Bytes()
	var rel uint64
	rel += uint64(len(seekBytes))
	infoPos := rel
	rel += uint64(len(info.Bytes()))
	tracksPos := rel
	rel += uint64(len(tracks.Bytes()))
	chaptersPos := rel
	rel += uint64(len(chapters.Bytes()))
	clusterPos := rel

	setSeekPosition(seekHead, IDInfo, infoPos)
	setSeekPosition(seekHead, IDTracks, tracksPos)
	setSeekPosition(seekHead, IDChapters, chaptersPos)

	cuePositions := ebml.NewContainer(IDCueTrackPositions,
		ebml.NewUintNode(IDCueClusterPosition, clusterPos))
	cuePoint := ebml.NewContainer(IDCuePoint, cuePositions)
	cues := ebml.NewContainer(IDCues, cuePoint)

	payload := bytes.Join([][]byte{
		seekHead.Bytes(), info.Bytes(), tracks.Bytes(), chapters.Bytes(), cluster, cues.Bytes(),
	}, nil)
	segment := ebml.ElementBytes(IDSegment, payload)
	header := ebml.ElementBytes(IDEBML, []byte("test-ebml-header"))
	return append(header, segment...)
}

func buildSeekHeadWithFixedPositions() *ebml.Node {
	seekFor := func(id ebml.ID) *ebml.Node {
		seekID := &ebml.Node{ID: IDSeekID, Payload: ebml.EncodeID(id)}
		position := &ebml.Node{ID: IDSeekPosition}
		setFixedUint(&ebml.Node{ID: IDSeek}, IDSeekPosition, 0)
		seek := ebml.NewContainer(IDSeek, seekID, position)
		setFixedUint(seek, IDSeekPosition, 0)
		return seek
	}
	return ebml.NewContainer(IDSeekHead, seekFor(IDInfo), seekFor(IDTracks), seekFor(IDChapters))
}

func setSeekPosition(seekHead *ebml.Node, target ebml.ID, position uint64) {
	for _, seek := range seekHead.All(IDSeek) {
		idNode := seek.First(IDSeekID)
		if idNode == nil {
			continue
		}
		raw := idFromBytes(idNode.Payload)
		if ebml.ID(raw) == target {
			setFixedUint(seek, IDSeekPosition, position)
		}
	}
}

func assertIndexTargets(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	segmentStart, header, err := findSegment(file)
	if err != nil {
		t.Fatal(err)
	}
	segmentData := data[segmentStart+int64(header.HeaderLength):]
	children, err := ebml.ParseChildren(segmentData, Containers)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[ebml.ID]int{}
	cursor := 0
	for _, child := range children {
		positions[child.ID] = cursor
		cursor += len(child.Bytes())
	}
	seekHead := childrenByID(children, IDSeekHead)
	if seekHead == nil {
		t.Fatal("missing SeekHead")
	}
	for _, seek := range seekHead.All(IDSeek) {
		idNode := seek.First(IDSeekID)
		posNode := seek.First(IDSeekPosition)
		target := ebml.ID(idFromBytes(idNode.Payload))
		got := ebml.ReadUint(posNode.Payload)
		if want := uint64(positions[target]); got != want {
			t.Fatalf("SeekPosition %x = %d, want %d", target, got, want)
		}
	}
	cues := childrenByID(children, IDCues)
	posNode := cues.First(IDCuePoint).First(IDCueTrackPositions).First(IDCueClusterPosition)
	got := ebml.ReadUint(posNode.Payload)
	if want := uint64(positions[testClusterID]); got != want {
		t.Fatalf("CueClusterPosition = %d, want %d", got, want)
	}
}

func childrenByID(nodes []*ebml.Node, id ebml.ID) *ebml.Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
	}
	return nil
}

var _ = binary.BigEndian
