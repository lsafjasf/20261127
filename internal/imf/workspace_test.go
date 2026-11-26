package imf

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	fixMain = "../../testdata/main-imp"
	fixSupp = "../../testdata/supp-imp"

	idVideo    = "11111111-1111-4111-8111-111111111111"
	idAudio    = "22222222-2222-4222-8222-222222222222"
	idLogo     = "33333333-3333-4333-8333-333333333333"
	idGhost    = "44444444-4444-4444-8444-444444444444"
	idSub      = "55555555-5555-4555-8555-555555555555"
	idDangling = "66666666-6666-4666-8666-666666666666"
	idMainCPL  = "cccccccc-0000-4000-8000-000000000003"
	idSuppCPL  = "cccccccc-0000-4000-8000-000000000013"
	idOPL      = "dddddddd-0000-4000-8000-000000000004"
)

func loadFixtureWorkspace(t *testing.T) *Workspace {
	t.Helper()
	mainPkg, err := LoadPackage(fixMain, "main", RoleMain)
	if err != nil {
		t.Fatalf("load main: %v", err)
	}
	suppPkg, err := LoadPackage(fixSupp, "supp-1", RoleSupplement)
	if err != nil {
		t.Fatalf("load supp: %v", err)
	}
	ws := NewWorkspace(mainPkg, suppPkg)
	ws.BuildRegistry()
	return ws
}

func TestLoadFixturePackages(t *testing.T) {
	ws := loadFixtureWorkspace(t)
	if len(ws.Packages[0].CPLs) != 1 || len(ws.Packages[0].OPLs) != 1 {
		t.Fatalf("main should have 1 CPL and 1 OPL, got cpl=%d opl=%d",
			len(ws.Packages[0].CPLs), len(ws.Packages[0].OPLs))
	}
	if len(ws.Packages[1].CPLs) != 1 {
		t.Fatalf("supp should have 1 CPL")
	}
	opl := ws.Packages[0].OPLs[0]
	if opl.ID != idOPL || len(opl.CPLRefs) != 1 || opl.CPLRefs[0] != idMainCPL {
		t.Fatalf("OPL inheritance ref wrong: %+v", opl)
	}
	if len(opl.TrackRefs) != 1 {
		t.Fatalf("OPL virtual track ref not parsed: %+v", opl.TrackRefs)
	}
}

func TestMergedPlaylistKeepsSubtitleTrack(t *testing.T) {
	ws := loadFixtureWorkspace(t)
	pl, err := ws.BuildPlaylist(MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pl.BaseCPL != idMainCPL {
		t.Fatalf("OPL should select main CPL as base, got %s", pl.BaseCPL)
	}
	if pl.SelectedOPL == nil || pl.SelectedOPL.ID != idOPL {
		t.Fatalf("OPL layer missing: %+v", pl.SelectedOPL)
	}
	if len(pl.SourceCPLs) != 2 {
		t.Fatalf("expected 2 source CPLs, got %d", len(pl.SourceCPLs))
	}
	tracks := allTracks(pl)
	if len(tracks) != 3 {
		t.Fatalf("expected image/audio/subtitle tracks, got %d: %v", len(tracks), trackNames(tracks))
	}
	var sub *Track
	for _, tr := range tracks {
		if tr.Kind == "subtitle" {
			sub = tr
		}
	}
	if sub == nil {
		t.Fatalf("subtitle track lost after merge: %v", trackNames(tracks))
	}
	if len(sub.Resources) != 2 {
		t.Fatalf("subtitle resources = %d, want 2", len(sub.Resources))
	}

	good := sub.Resources[0].Resolved
	if good == nil || !good.Found || !good.FileExists {
		t.Fatalf("present subtitle asset not resolved: %+v", good)
	}
	if !strings.HasSuffix(good.AbsolutePath, filepath.Join("supp-imp", "SUBTITLE_5555.mxf")) {
		t.Fatalf("subtitle should resolve into supplement package, got %s", good.AbsolutePath)
	}
	if sub.Resources[1].TrackFileID != idDangling {
		t.Fatalf("second subtitle resource should be dangling fixture")
	}
	if r := sub.Resources[1].Resolved; r == nil || r.Found {
		t.Fatalf("dangling subtitle ref should be unresolved, got %+v", r)
	}
}

func TestMainOnlyPlaylistHasNoSubtitle(t *testing.T) {
	// Encodes the reported production bug: resolving only against the main
	// package makes the subtitle track/reference disappear from the playlist.
	mainPkg, err := LoadPackage(fixMain, "main", RoleMain)
	if err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace(mainPkg)
	ws.BuildRegistry()
	pl, err := ws.BuildPlaylist(MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range allTracks(pl) {
		if tr.Kind == "subtitle" {
			t.Fatalf("main-only fixture must not contain subtitle track")
		}
	}
	if r := ws.Resolve(idSub); r.Found {
		t.Fatalf("subtitle asset must not resolve from main package alone")
	}
}

func TestMissingAndDuplicateReport(t *testing.T) {
	ws := loadFixtureWorkspace(t)
	rep := ws.Report

	missingKinds := map[string]bool{}
	missingIDs := map[string]bool{}
	for _, m := range rep.Missing {
		missingKinds[m.Kind] = true
		missingIDs[m.AssetID] = true
	}
	if !missingKinds["unresolved_reference"] || !missingIDs[idDangling] {
		t.Fatalf("expected unresolved_reference for %s, got %+v", idDangling, rep.Missing)
	}
	if !missingKinds["file_not_found"] || !missingIDs[idGhost] {
		t.Fatalf("expected file_not_found for %s, got %+v", idGhost, rep.Missing)
	}

	dupByAsset := map[string]DuplicateIssue{}
	for _, d := range rep.Duplicates {
		if d.AssetID != "" {
			dupByAsset[d.AssetID] = d
		}
	}
	audioDup, ok := dupByAsset[idAudio]
	if !ok || audioDup.Severity != SeverityIdentical {
		t.Fatalf("audio should be identical cross-package duplicate, got %+v", dupByAsset)
	}
	logoDup, ok := dupByAsset[idLogo]
	if !ok || logoDup.Severity != SeverityConflict {
		t.Fatalf("logo should be conflict duplicate, got %+v", logoDup)
	}
	if rep.Summary.DuplicateConflicts != 1 || rep.Summary.MissingTotal != 2 {
		t.Fatalf("unexpected summary: %+v", rep.Summary)
	}
	if !rep.HasBlockingFindings() {
		t.Fatalf("conflict + missing should be blocking under strict mode")
	}
}

func TestRepeatedInheritedResourceIsDeduped(t *testing.T) {
	mkCPL := func(id, trackID, resID, tfID string) *CPL {
		return &CPL{
			ID: id, PackageLabel: "p",
			Segments: []*Segment{{ID: "seg1", Tracks: []*Track{{
				SequenceName: "MainAudioSequence", Kind: "audio", TrackID: trackID,
				Resources: []*Resource{{ID: resID, TrackFileID: tfID, RepeatCount: 1}},
			}}}},
		}
	}
	base := &Package{Label: "main", Dir: ".", CPLs: []*CPL{mkCPL(idMainCPL, "vt-a", "res-1", idAudio)}}
	supp := &Package{Label: "supp", Dir: ".", CPLs: []*CPL{mkCPL(idSuppCPL, "vt-a", "res-1", idAudio)}}
	ws := NewWorkspace(base, supp)
	ws.BuildRegistry()
	pl, err := ws.BuildPlaylist(MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tr := allTracks(pl)[0]
	if len(tr.Resources) != 1 {
		t.Fatalf("identical inherited resource should dedupe, got %d", len(tr.Resources))
	}
}

func TestWriteOutputsAndSourceImmutability(t *testing.T) {
	ws := loadFixtureWorkspace(t)
	pl, err := ws.BuildPlaylist(MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	beforeMain := hashTree(t, fixMain)
	beforeSupp := hashTree(t, fixSupp)

	out := t.TempDir()
	if err := WriteOutputs(out, pl, ws.Report, WriteOptions{CopyAssets: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"playlist.json", "report.json", "report.txt", "CPL_final.xml"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "SUBTITLE_5555.mxf")); err != nil {
		t.Fatalf("subtitle asset should be copied: %v", err)
	}
	if after := hashTree(t, fixMain); after != beforeMain {
		t.Fatalf("main package changed by run:\n%s\nvs\n%s", beforeMain, after)
	}
	if after := hashTree(t, fixSupp); after != beforeSupp {
		t.Fatalf("supp package changed by run:\n%s\nvs\n%s", beforeSupp, after)
	}

	// The generated CPL must parse and retain the subtitle track.
	cpl2, err := parseCPLFile(filepath.Join(out, "CPL_final.xml"), "out")
	if err != nil {
		t.Fatalf("generated CPL not parseable: %v", err)
	}
	var names []string
	for _, s := range cpl2.Segments {
		for _, tr := range s.Tracks {
			names = append(names, tr.SequenceName)
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "MainAudioSequence,MainImageSequence,MainSubtitleSequence" {
		t.Fatalf("round-trip CPL tracks wrong: %v", names)
	}
}

func TestEnsureOutputSeparate(t *testing.T) {
	mainAbs, _ := filepath.Abs(fixMain)
	if err := EnsureOutputSeparate(filepath.Join(fixMain, "out"), []string{fixMain}); err == nil {
		t.Fatalf("out inside package must be rejected")
	}
	if err := EnsureOutputSeparate(fixMain, []string{fixMain}); err == nil {
		t.Fatalf("out == package must be rejected")
	}
	if err := EnsureOutputSeparate(t.TempDir(), []string{fixMain}); err != nil {
		t.Fatalf("separate temp out should be allowed: %v", err)
	}
	_ = mainAbs
}

func TestFilenameUUIDFallbackWithoutAssetMap(t *testing.T) {
	dir := t.TempDir()
	tfID := "77777777-7777-4777-8777-777777777777"
	cplXML := `<?xml version="1.0" encoding="UTF-8"?>
<CompositionPlaylist xmlns="http://www.smpte-ra.org/schemas/2067-3/2016" xmlns:cc="http://www.smpte-ra.org/schemas/2067-2/2016" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Id>urn:uuid:cccccccc-0000-4000-8000-000000000099</Id>
  <EditRate>24 1</EditRate>
  <SegmentList><Segment><Id>urn:uuid:eeeeeeee-0000-4000-8000-000000000099</Id>
    <SequenceList><cc:MainSubtitleSequence>
      <TrackId>urn:uuid:a0000009-0000-4000-8000-000000000009</TrackId>
      <ResourceList><Resource xsi:type="TrackFileResourceType">
        <Id>urn:uuid:d0000009-0000-4000-8000-000000000009</Id>
        <TrackFileId>urn:uuid:` + tfID + `</TrackFileId>
        <IntrinsicDuration>24</IntrinsicDuration>
      </Resource></ResourceList>
    </cc:MainSubtitleSequence></SequenceList>
  </Segment></SegmentList>
</CompositionPlaylist>`
	if err := os.WriteFile(filepath.Join(dir, "CPL.xml"), []byte(cplXML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tfID+".mxf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadPackage(dir, "bare", RoleExtra)
	if err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace(pkg)
	rep := ws.BuildRegistry()
	for _, m := range rep.Missing {
		if m.Kind == "unresolved_reference" {
			t.Fatalf("UUID-named file should resolve without ASSETMAP: %+v", rep.Missing)
		}
	}
	ra := ws.Resolve(tfID)
	if !ra.Found || !ra.FileExists {
		t.Fatalf("fallback resolution failed: %+v", ra)
	}
}

func TestDiscoverPackageDirs(t *testing.T) {
	root := t.TempDir()
	mainChild := filepath.Join(root, "main-pkg")
	emptyChild := filepath.Join(root, "not-a-pkg")
	if err := os.MkdirAll(mainChild, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emptyChild, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainChild, "ASSETMAP.xml"),
		[]byte(`<?xml version="1.0"?><AssetMap xmlns="x"><Id>urn:uuid:aaaaaaaa-0000-4000-8000-000000000001</Id></AssetMap>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LooksLikePackage(mainChild) {
		t.Fatalf("ASSETMAP child should look like a package")
	}
	if LooksLikePackage(emptyChild) {
		t.Fatalf("empty child should not look like a package")
	}
	dirs, err := DiscoverPackageDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != mainChild {
		t.Fatalf("expected only main-pkg, got %v", dirs)
	}
	if !LooksLikePackage(fixMain) || !LooksLikePackage(fixSupp) {
		t.Fatalf("fixture dirs should be recognized as packages")
	}
}

func allTracks(pl *Playlist) []*Track {
	var out []*Track
	for _, s := range pl.Segments {
		out = append(out, s.Tracks...)
	}
	return out
}

func trackNames(ts []*Track) []string {
	var n []string
	for _, t := range ts {
		n = append(n, t.SequenceName)
	}
	return n
}

func hashTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		b.WriteString(rel)
		b.WriteByte('=')
		b.WriteString(hex.EncodeToString(sum[:]))
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
