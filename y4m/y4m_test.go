package y4m

import (
	"bytes"
	"io"
	"testing"
)

func TestReader420AndTimestamps(t *testing.T) {
	var data bytes.Buffer
	data.WriteString("YUV4MPEG2 W4 H2 F10:1 Ip C420jpeg\n")
	for i := 0; i < 2; i++ {
		data.WriteString("FRAME\n")
		data.Write([]byte{byte(40 + i), byte(50 + i), byte(60 + i), byte(70 + i),
			byte(80 + i), byte(90 + i), byte(100 + i), byte(110 + i)})
		data.Write(make([]byte, 2*1*2))
	}
	reader, err := NewReader(&data)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Width() != 4 || reader.Height() != 2 || reader.FPS() != 10 {
		t.Fatalf("unexpected metadata: %dx%d @ %.2f", reader.Width(), reader.Height(), reader.FPS())
	}
	first, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Luma[0] != 40 || first.Timestamp != 0 {
		t.Fatalf("bad first frame: luma0=%d t=%.3f", first.Luma[0], first.Timestamp)
	}
	second, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if second.Luma[7] != 111 || second.Timestamp != 0.1 {
		t.Fatalf("bad second frame: luma7=%d t=%.3f", second.Luma[7], second.Timestamp)
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("got %v, want EOF", err)
	}
}

func TestReaderMonoAndRejectsInvalidInput(t *testing.T) {
	reader, err := NewReader(bytes.NewBufferString("YUV4MPEG2 W2 H1 F25:1 Cmono\nFRAME\n\x01\x02"))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if len(frame.Luma) != 2 || frame.Luma[1] != 2 {
		t.Fatalf("unexpected mono frame: %v", frame.Luma)
	}
	if _, err := NewReader(bytes.NewBufferString("not-a-y4m\n")); err == nil {
		t.Fatal("invalid input was accepted")
	}
	if _, err := NewReader(bytes.NewBufferString("YUV4MPEG2 W2 H1 F25:1 C420p10\n")); err == nil {
		t.Fatal("10-bit input was accepted")
	}
}
