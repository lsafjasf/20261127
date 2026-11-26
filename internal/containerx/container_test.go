package containerx

import (
	"path/filepath"
	"testing"
)

func TestMemberSetIndependentOfOrderAndNames(t *testing.T) {
	a := []Entry{{Name: "a.txt", Data: []byte("alpha contents")}, {Name: "b.bin", Data: []byte{1, 2, 3, 4}}}
	reordered := []Entry{a[1], a[0]}
	renamed := []Entry{{Name: "x.txt", Data: a[0].Data}, {Name: "y.bin", Data: a[1].Data}}

	s1 := MemberSetDigest(MembersFromEntries(a))
	s2 := MemberSetDigest(MembersFromEntries(reordered))
	s3 := MemberSetDigest(MembersFromEntries(renamed))
	if s1 != s2 || s1 != s3 {
		t.Fatal("memberset digest must ignore entry order and member names")
	}

	p1 := PathSetDigest(MembersFromEntries(a))
	p3 := PathSetDigest(MembersFromEntries(renamed))
	if p1 == p3 {
		t.Fatal("pathset digest must change when member names change")
	}
}

func TestZipAndTarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	entries := []Entry{
		{Name: "dir/note.txt", Data: []byte("hello evidence")},
		{Name: "blob.bin", Data: []byte{0, 1, 2, 3, 4, 5}},
	}
	zipPath := filepath.Join(dir, "c.zip")
	tarPath := filepath.Join(dir, "c.tar")
	if err := WriteZip(zipPath, entries); err != nil {
		t.Fatal(err)
	}
	if err := WriteTar(tarPath, entries); err != nil {
		t.Fatal(err)
	}
	_, zm, err := InspectFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	_, tm, err := InspectFile(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	if zm.MemberSetSHA256 != tm.MemberSetSHA256 {
		t.Fatal("zip and tar with same members must share memberset digest")
	}
	if len(zm.Members) != 2 || len(tm.Members) != 2 {
		t.Fatal("expected two members in each container")
	}

	format, _, read, err := ReadEntries(zipPath)
	if err != nil || format != "zip" {
		t.Fatalf("ReadEntries zip: format=%q err=%v", format, err)
	}
	if string(read[0].Data) == "" {
		t.Fatal("member bytes should be readable")
	}
}
