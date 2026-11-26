package mp4

import "testing"

func TestParseChildrenRejectsTruncation(t *testing.T) {
	data := []byte{0, 0, 0, 20, 'm', 'o', 'o', 'v', 1, 2}
	if _, err := ParseChildren(data); err == nil {
		t.Fatal("expected an invalid child box size")
	}
}

func TestFindBox(t *testing.T) {
	payload := make([]byte, 8)
	payload[0], payload[1], payload[2], payload[3] = 0, 0, 0, 8
	copy(payload[4:8], "hvcC")
	boxes, err := ParseChildren(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := Find(boxes, "hvcC"); got == nil || string(got.Type) != "hvcC" {
		t.Fatalf("box = %+v", got)
	}
	if _, err := ParseChildren([]byte{0, 0, 0, 20, 'm', 'o', 'o', 'v'}); err == nil {
		t.Fatal("expected an invalid box size")
	}
}
