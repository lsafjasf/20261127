package imf

import (
	"encoding/xml"
	"os"
	"strconv"
)

const (
	nsCPL3 = "http://www.smpte-ra.org/schemas/2067-3/2016"
	nsCC2  = "http://www.smpte-ra.org/schemas/2067-2/2016"
	nsXSI  = "http://www.w3.org/2001/XMLSchema-instance"
)

// WriteMergedCPL writes the final playlist as a CPL-style XML document.
// Typed sequence elements keep their original local names (including
// MainSubtitleSequence and friends), so subtitle tracks are never dropped.
func WriteMergedCPL(path string, pl *Playlist) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")

	proc := xml.ProcInst{Target: "xml", Inst: []byte(`version="1.0" encoding="UTF-8"`)}
	if err := enc.EncodeToken(proc); err != nil {
		return err
	}
	if err := enc.EncodeToken(xml.CharData([]byte("\n"))); err != nil {
		return err
	}

	root := xml.StartElement{Name: xml.Name{Local: "CompositionPlaylist"}, Attr: []xml.Attr{
		{Name: xml.Name{Local: "xmlns"}, Value: nsCPL3},
		{Name: xml.Name{Local: "xmlns:cc"}, Value: nsCC2},
		{Name: xml.Name{Local: "xmlns:xsi"}, Value: nsXSI},
	}}
	if err := enc.EncodeToken(root); err != nil {
		return err
	}
	textEl(enc, "Id", URN(pl.OutputCPLID))
	textEl(enc, "AnnotationText", pl.Annotation)
	if pl.EditRate != "" {
		textEl(enc, "EditRate", pl.EditRate)
	}

	sl := xml.StartElement{Name: xml.Name{Local: "SegmentList"}}
	enc.EncodeToken(sl)
	for _, seg := range pl.Segments {
		writeSegment(enc, seg)
	}
	enc.EncodeToken(xml.EndElement{Name: sl.Name})

	enc.EncodeToken(xml.EndElement{Name: root.Name})
	enc.Flush()
	_, _ = f.Write([]byte("\n"))
	return f.Close()
}

func writeSegment(enc *xml.Encoder, seg *Segment) {
	st := xml.StartElement{Name: xml.Name{Local: "Segment"}}
	enc.EncodeToken(st)
	if seg.ID != "" {
		textEl(enc, "Id", URN(seg.ID))
	}
	seqList := xml.StartElement{Name: xml.Name{Local: "SequenceList"}}
	enc.EncodeToken(seqList)
	for _, tr := range seg.Tracks {
		writeTrack(enc, tr)
	}
	enc.EncodeToken(xml.EndElement{Name: seqList.Name})
	enc.EncodeToken(xml.EndElement{Name: st.Name})
}

func writeTrack(enc *xml.Encoder, tr *Track) {
	name := tr.SequenceName
	if name == "" {
		name = "TrackSequence"
	}
	// Emit core-constraint typed sequences with the conventional cc prefix.
	elName := name
	if isCCSequence(name) {
		elName = "cc:" + name
	}
	st := xml.StartElement{Name: xml.Name{Local: elName}}
	enc.EncodeToken(st)
	if tr.SequenceID != "" {
		textEl(enc, "Id", URN(tr.SequenceID))
	}
	if tr.TrackID != "" {
		textEl(enc, "TrackId", URN(tr.TrackID))
	}
	rl := xml.StartElement{Name: xml.Name{Local: "ResourceList"}}
	enc.EncodeToken(rl)
	for _, res := range tr.Resources {
		writeResource(enc, res)
	}
	enc.EncodeToken(xml.EndElement{Name: rl.Name})
	enc.EncodeToken(xml.EndElement{Name: st.Name})
}

func writeResource(enc *xml.Encoder, res *Resource) {
	st := xml.StartElement{Name: xml.Name{Local: "Resource"}, Attr: []xml.Attr{
		{Name: xml.Name{Local: "xsi:type"}, Value: "TrackFileResourceType"},
	}}
	enc.EncodeToken(st)
	if res.ID != "" {
		textEl(enc, "Id", URN(res.ID))
	}
	if res.TrackFileID != "" {
		textEl(enc, "TrackFileId", URN(res.TrackFileID))
	}
	if res.EditRate != "" {
		textEl(enc, "EditRate", res.EditRate)
	}
	if res.IntrinsicDuration > 0 {
		textEl(enc, "IntrinsicDuration", strconv.FormatInt(res.IntrinsicDuration, 10))
	}
	if res.EntryPoint > 0 {
		textEl(enc, "EntryPoint", strconv.FormatInt(res.EntryPoint, 10))
	}
	if res.SourceDuration > 0 {
		textEl(enc, "SourceDuration", strconv.FormatInt(res.SourceDuration, 10))
	}
	if res.RepeatCount > 1 {
		textEl(enc, "RepeatCount", strconv.Itoa(res.RepeatCount))
	}
	enc.EncodeToken(xml.EndElement{Name: st.Name})
}

func textEl(enc *xml.Encoder, name, value string) {
	if value == "" {
		return
	}
	st := xml.StartElement{Name: xml.Name{Local: name}}
	_ = enc.EncodeToken(st)
	_ = enc.EncodeToken(xml.CharData([]byte(value)))
	_ = enc.EncodeToken(xml.EndElement{Name: st.Name})
}

func isCCSequence(name string) bool {
	switch classifyTrackKind(name) {
	case "image", "audio", "subtitle", "data":
		return true
	default:
		return false
	}
}
