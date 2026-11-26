package matroska

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mkvidentity/ebml"
	"mkvidentity/identity"
)

type RewriteResult struct {
	OutputPath           string   `json:"output_path"`
	BytesWritten         int64    `json:"bytes_written"`
	TracksPatched        int      `json:"tracks_patched"`
	ChaptersReplaced     int      `json:"chapters_replaced"`
	ChapterRefsRewritten int      `json:"chapter_refs_rewritten"`
	SeekEntriesFixed     int      `json:"seek_entries_fixed"`
	CuePositionsFixed    int      `json:"cue_positions_fixed"`
	MissingTrackUIDs     []uint64 `json:"missing_track_uids,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

type topElement struct {
	header      ebml.Header
	headerStart int64
	dataStart   int64
	end         int64
	node        *ebml.Node
	dirty       bool
	replacement []byte
	inserted    bool
}

// RewriteFile writes a new Matroska file with track patches and/or replacement
// chapters. Media elements are copied byte-for-byte; no external muxer is
// executed.
func RewriteFile(inputPath, outputPath string, patch identity.Patch) (*RewriteResult, error) {
	if inputPath == outputPath {
		return nil, fmt.Errorf("output path must differ from input path")
	}
	in, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return nil, err
	}
	fileEnd := info.Size()

	segmentStart, segmentHeader, err := findSegment(in)
	if err != nil {
		return nil, err
	}
	segmentDataStart := segmentStart + int64(segmentHeader.HeaderLength)
	segmentEnd := fileEnd
	if !segmentHeader.UnknownSize {
		segmentEnd = segmentDataStart + int64(segmentHeader.Size)
	}

	elements, err := scanTopElements(in, segmentHeader, segmentDataStart, segmentEnd)
	if err != nil {
		return nil, err
	}
	result := &RewriteResult{OutputPath: outputPath}
	elements, err = applyPatchToElements(elements, patch, result)
	if err != nil {
		return nil, err
	}
	if err := prepareIndexUpdates(elements); err != nil {
		return nil, err
	}

	layout := layoutElements(elements, segmentDataStart)
	updateIndexPositions(elements, layout, segmentDataStart, result)

	out, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".tmp-*")
	if err != nil {
		return nil, err
	}
	defer out.Close()
	tmpPath := out.Name()
	defer os.Remove(tmpPath)

	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.CopyN(out, in, segmentStart); err != nil {
		return nil, err
	}
	if err := writeSegmentHeader(out, segmentHeader, layout.payloadLength); err != nil {
		return nil, err
	}
	for _, element := range elements {
		switch {
		case len(element.replacement) > 0:
			if _, err := out.Write(element.replacement); err != nil {
				return nil, err
			}
		case element.dirty && element.node != nil:
			if _, err := out.Write(element.node.Bytes()); err != nil {
				return nil, err
			}
		case element.inserted:
			return nil, fmt.Errorf("inserted element %x has no replacement", element.header.ID)
		default:
			if _, err := in.Seek(element.headerStart, io.SeekStart); err != nil {
				return nil, err
			}
			if _, err := io.CopyN(out, in, element.end-element.headerStart); err != nil {
				return nil, err
			}
		}
	}
	if segmentEnd < fileEnd {
		if _, err := in.Seek(segmentEnd, io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := io.CopyN(out, in, fileEnd-segmentEnd); err != nil {
			return nil, err
		}
	}
	endOffset, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	result.BytesWritten = endOffset
	if err := out.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return nil, err
	}
	return result, nil
}

func findSegment(in *os.File) (int64, ebml.Header, error) {
	for {
		start, err := in.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, ebml.Header{}, err
		}
		header, err := ebml.ReadHeader(in)
		if err != nil {
			return 0, ebml.Header{}, err
		}
		if header.ID == IDSegment {
			return start, header, nil
		}
		if header.UnknownSize {
			return 0, ebml.Header{}, fmt.Errorf("top-level element %x has unsupported unknown size", header.ID)
		}
		if _, err := in.Seek(start+int64(header.HeaderLength)+int64(header.Size), io.SeekStart); err != nil {
			return 0, ebml.Header{}, err
		}
	}
}

func scanTopElements(in *os.File, segment ebml.Header, dataStart, segmentEnd int64) ([]*topElement, error) {
	var elements []*topElement
	offset := dataStart
	for offset < segmentEnd {
		if _, err := in.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		header, err := ebml.ReadHeader(in)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
		headerStart := offset
		elementDataStart := offset + int64(header.HeaderLength)
		end := segmentEnd
		if !header.UnknownSize {
			end = elementDataStart + int64(header.Size)
		}
		element := &topElement{
			header: header, headerStart: headerStart, dataStart: elementDataStart, end: end,
		}
		if header.ID == IDTracks || header.ID == IDChapters || header.ID == IDSeekHead || header.ID == IDCues {
			size := end - elementDataStart
			payload := make([]byte, size)
			if _, err := io.ReadFull(in, payload); err != nil {
				return nil, err
			}
			node, err := parseSingle(header.ID, payload)
			if err != nil {
				return nil, fmt.Errorf("parse %x: %w", header.ID, err)
			}
			element.node = node
		}
		elements = append(elements, element)
		offset = end
		if header.UnknownSize {
			break
		}
	}
	return elements, nil
}

func writeSegmentHeader(out *os.File, header ebml.Header, payloadLength int64) error {
	if _, err := out.Write(ebml.EncodeID(IDSegment)); err != nil {
		return err
	}
	if header.UnknownSize {
		_, err := out.Write(ebml.EncodeUnknownSize(header.SizeLength))
		return err
	}
	_, err := out.Write(ebml.EncodeSize(uint64(payloadLength)))
	return err
}
