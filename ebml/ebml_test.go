package ebml

import "testing"

func TestParseUintElement(t *testing.T) {
	data := []byte{0x53, 0xc0, 0x81, 0x01}
	els, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(els) != 1 || els[0].ID != IDAlphaMode {
		t.Fatalf("elements=%+v", els)
	}
	if els[0].Int() != 1 || els[0].HeaderSize != 3 {
		t.Fatalf("alpha element=%+v", els[0])
	}
}

func TestParseNestedCrop(t *testing.T) {
	data := []byte{
		0xe0, 0x88,
		0x54, 0xcc, 0x81, 0x08,
		0x54, 0xdd, 0x81, 0x08,
	}
	els, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	video := els[0]
	if got := video.Find(IDCropLeft).Int(); got != 8 {
		t.Fatalf("left=%d", got)
	}
	if got := video.Find(IDCropRight).Int(); got != 8 {
		t.Fatalf("right=%d", got)
	}
}

func TestTruncatedElement(t *testing.T) {
	if _, err := Parse([]byte{0xb0, 0x84, 0x01}); err == nil {
		t.Fatal("expected truncation error")
	}
}
