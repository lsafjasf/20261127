package matroska

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"mkvidentity/ebml"
	"mkvidentity/identity"
	"mkvidentity/timepoint"
)

const defaultTimecodeScaleNS uint64 = 1_000_000

// ReadInventoryPath scans only EBML headers and metadata elements. Media
// payload and Cluster bodies are seeked past, so memory use does not scale
// with movie size.
func ReadInventoryPath(path string) (*identity.Inventory, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, tracks, chapters, err := scanMetadata(file)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	inventory := InventoryFromNodes(info, tracks, chapters)
	inventory.Path = path
	return inventory, nil
}

type metadataNodes struct {
	info     *ebml.Node
	tracks   *ebml.Node
	chapters *ebml.Node
}

func scanMetadata(file *os.File) (*ebml.Node, *ebml.Node, *ebml.Node, error) {
	for {
		header, err := ebml.ReadHeader(file)
		if err == io.EOF {
			return nil, nil, nil, fmt.Errorf("no Matroska Segment found")
		}
		if err != nil {
			return nil, nil, nil, err
		}
		payloadStart, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, nil, nil, err
		}
		if header.ID != IDSegment {
			if _, err := file.Seek(int64(header.Size), io.SeekCurrent); err != nil {
				return nil, nil, nil, err
			}
			continue
		}

		segmentEnd := int64(-1)
		if !header.UnknownSize {
			segmentEnd = payloadStart + int64(header.Size)
		}
		var meta metadataNodes
		for {
			offset, _ := file.Seek(0, io.SeekCurrent)
			if segmentEnd >= 0 && offset >= segmentEnd {
				break
			}
			child, err := ebml.ReadHeader(file)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, nil, err
			}
			dataStart, err := file.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, nil, nil, err
			}
			var nextOffset int64
			if child.UnknownSize {
				if segmentEnd < 0 {
					return meta.info, meta.tracks, meta.chapters, nil
				}
				nextOffset = segmentEnd
			} else {
				nextOffset = dataStart + int64(child.Size)
			}

			switch child.ID {
			case IDInfo, IDTracks, IDChapters:
				size := nextOffset - dataStart
				payload := make([]byte, size)
				if _, err := io.ReadFull(file, payload); err != nil {
					return nil, nil, nil, err
				}
				node, err := parseSingle(child.ID, payload)
				if err != nil {
					return nil, nil, nil, err
				}
				switch child.ID {
				case IDInfo:
					meta.info = node
				case IDTracks:
					meta.tracks = node
				case IDChapters:
					meta.chapters = node
				}
			default:
				// Media clusters and attachments are skipped by header size.
			}
			if _, err := file.Seek(nextOffset, io.SeekStart); err != nil {
				return nil, nil, nil, err
			}
		}
		return meta.info, meta.tracks, meta.chapters, nil
	}
}

func parseSingle(id ebml.ID, payload []byte) (*ebml.Node, error) {
	children, err := ebml.ParseChildren(payload, Containers)
	if err != nil {
		return nil, err
	}
	return &ebml.Node{ID: id, Children: children}, nil
}

func InventoryFromNodes(info, tracks, chapters *ebml.Node) *identity.Inventory {
	inv := &identity.Inventory{TimecodeScaleNS: defaultTimecodeScaleNS}
	if info != nil {
		if scale, ok := info.UintValue(IDTimecodeScale); ok && scale != 0 {
			inv.TimecodeScaleNS = scale
		}
		if durationNode := info.First(IDDuration); durationNode != nil {
			if seconds, err := ebml.ReadFloat(durationNode.Payload); err == nil {
				ns := uint64(seconds * float64(inv.TimecodeScaleNS))
				inv.DurationNS = &ns
			}
		}
	}
	if tracks != nil {
		for index, entry := range tracks.All(IDTrackEntry) {
			inv.Tracks = append(inv.Tracks, readTrack(entry, index+1))
		}
	}
	if chapters != nil {
		readChapters(chapters, inv)
	}
	for i := range inv.Chapters {
		inv.Chapters[i].LegacyLine = i + 1
	}
	return inv
}

func readTrack(node *ebml.Node, index int) identity.Track {
	track := identity.Track{LegacyLine: index, Enabled: true, Default: true}
	track.UID, _ = node.UintValue(IDTrackUID)
	track.Number, _ = node.UintValue(IDTrackNumber)
	if typ, ok := node.UintValue(IDTrackType); ok {
		track.Type = identity.TrackType(typ)
	}
	track.CodecID, _ = node.StringValue(IDCodecID)
	track.Language, _ = node.StringValue(IDLanguage)
	track.LanguageIETF, _ = node.StringValue(IDLanguageIETF)
	track.Name, _ = node.StringValue(IDName)
	if value, ok := node.UintValue(IDFlagDefault); ok {
		track.DefaultSet, track.Default = true, value != 0
	}
	if value, ok := node.UintValue(IDFlagEnabled); ok {
		track.Enabled = value != 0
	}
	if value, ok := node.UintValue(IDFlagForced); ok {
		track.Forced = value != 0
	}
	track.Fingerprint = trackFingerprint(node, track)
	return track
}

func trackFingerprint(node *ebml.Node, track identity.Track) string {
	hasher := sha256.New()
	fmt.Fprintf(hasher, "type=%d\ncodec=%s\n", track.Type, track.CodecID)
	if private := node.First(IDCodecPrivate); private != nil {
		sum := sha256.Sum256(private.Payload)
		fmt.Fprintf(hasher, "private=%s\n", hex.EncodeToString(sum[:]))
	}
	if audio := node.First(IDAudio); audio != nil {
		if channels, ok := audio.UintValue(IDChannels); ok {
			fmt.Fprintf(hasher, "channels=%d\n", channels)
		}
		if rateNode := audio.First(IDSamplingFrequency); rateNode != nil {
			if rate, err := ebml.ReadFloat(rateNode.Payload); err == nil {
				fmt.Fprintf(hasher, "rate=%.3f\n", rate)
			}
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))[:16]
}

func readChapters(chapters *ebml.Node, inv *identity.Inventory) {
	for _, editionNode := range chapters.All(IDEditionEntry) {
		edition := identity.Edition{}
		edition.UID, _ = editionNode.UintValue(IDEditionUID)
		if value, ok := editionNode.UintValue(IDEditionDefault); ok {
			edition.Default = value != 0
		}
		if value, ok := editionNode.UintValue(IDEditionOrdered); ok {
			edition.Ordered = value != 0
		}
		atoms := editionNode.All(IDChapterAtom)
		for index, atom := range atoms {
			readAtom(atom, &edition, 0, index+1, inv)
		}
		inv.Editions = append(inv.Editions, edition)
	}
}

func readAtom(node *ebml.Node, edition *identity.Edition, parentUID uint64, ordinal int, inv *identity.Inventory) {
	chapter := identity.Chapter{
		EditionUID: edition.UID, ParentUID: parentUID, Ordinal: ordinal, Enabled: true,
	}
	chapter.UID, _ = node.UintValue(IDChapterUID)
	chapter.StringUID, _ = node.StringValue(IDChapterStringUID)
	if raw, ok := node.UintValue(IDChapterTimeStart); ok {
		chapter.Start = timepoint.Resolved(raw)
	} else {
		chapter.Start = timepoint.Unresolved("", timepoint.StatusEmpty, "missing ChapterTimeStart")
	}
	if raw, ok := node.UintValue(IDChapterTimeEnd); ok {
		end := timepoint.Resolved(raw)
		chapter.End = &end
	}
	if value, ok := node.UintValue(IDChapterTrackUID); ok {
		chapter.TrackUIDs = append(chapter.TrackUIDs, value)
	}
	if trackContainer := node.First(IDChapterTrack); trackContainer != nil {
		for _, refNode := range trackContainer.All(IDChapterTrackUID) {
			chapter.TrackUIDs = append(chapter.TrackUIDs, ebml.ReadUint(refNode.Payload))
		}
	}
	for _, display := range node.All(IDChapterDisplay) {
		item := identity.ChapterDisplay{}
		item.String, _ = display.StringValue(IDChapString)
		item.Language, _ = display.StringValue(IDChapLanguage)
		item.LanguageIETF, _ = display.StringValue(IDChapLanguageIETF)
		chapter.Displays = append(chapter.Displays, item)
	}
	if value, ok := node.UintValue(IDChapterUID); ok {
		_ = value
	}
	if enabled, ok := node.UintValue(IDChapterEnabled); ok {
		chapter.Enabled = enabled != 0
	}

	inv.Chapters = append(inv.Chapters, chapter)
	if edition.UID != 0 && parentUID == 0 {
		edition.ChapterUIDs = append(edition.ChapterUIDs, chapter.UID)
	}

	children := node.All(IDChapterAtom)
	for index, child := range children {
		readAtom(child, edition, chapter.UID, index+1, inv)
	}
}
