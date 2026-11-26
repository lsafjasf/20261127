package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadStereo16BitWAV(t *testing.T) {
	samples := []int16{
		-32768, 32767,
		0, -16384,
		16384, 8192,
	}
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	dataBytes := len(samples) * 2
	binary.Write(&buffer, binary.LittleEndian, uint32(36+dataBytes))
	buffer.WriteString("WAVE")
	buffer.WriteString("fmt ")
	binary.Write(&buffer, binary.LittleEndian, uint32(16))
	binary.Write(&buffer, binary.LittleEndian, uint16(pcmFormat))
	binary.Write(&buffer, binary.LittleEndian, uint16(2))
	binary.Write(&buffer, binary.LittleEndian, uint32(8000))
	binary.Write(&buffer, binary.LittleEndian, uint32(32000))
	binary.Write(&buffer, binary.LittleEndian, uint16(4))
	binary.Write(&buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	binary.Write(&buffer, binary.LittleEndian, uint32(dataBytes))
	for _, sample := range samples {
		binary.Write(&buffer, binary.LittleEndian, sample)
	}

	audio, err := ReadWAV(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if audio.SampleRate != 8000 || len(audio.Left) != 3 || len(audio.Right) != 3 {
		t.Fatalf("unexpected WAV layout: rate=%d left=%d right=%d", audio.SampleRate, len(audio.Left), len(audio.Right))
	}
	if audio.Left[0] >= -0.999 || audio.Right[0] < 0.999 {
		t.Fatalf("stereo channels or normalization wrong: L=%v R=%v", audio.Left[0], audio.Right[0])
	}
	if audio.Left[1] != 0 || audio.Right[1] >= -0.499 || audio.Right[1] <= -0.501 {
		t.Fatalf("unexpected decoded samples: L=%v R=%v", audio.Left[1], audio.Right[1])
	}
}

func TestRejectsMonoWAV(t *testing.T) {
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	binary.Write(&buffer, binary.LittleEndian, uint32(40))
	buffer.WriteString("WAVE")
	buffer.WriteString("fmt ")
	binary.Write(&buffer, binary.LittleEndian, uint32(16))
	binary.Write(&buffer, binary.LittleEndian, uint16(pcmFormat))
	binary.Write(&buffer, binary.LittleEndian, uint16(1))
	binary.Write(&buffer, binary.LittleEndian, uint32(8000))
	binary.Write(&buffer, binary.LittleEndian, uint32(16000))
	binary.Write(&buffer, binary.LittleEndian, uint16(2))
	binary.Write(&buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	binary.Write(&buffer, binary.LittleEndian, uint32(4))
	binary.Write(&buffer, binary.LittleEndian, []int16{1, 2})

	if _, err := ReadWAV(&buffer); err == nil {
		t.Fatal("expected mono input to be rejected")
	}
}
