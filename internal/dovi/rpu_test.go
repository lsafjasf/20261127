package dovi_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/testmedia"
)

func TestCRC32MPEG2Vector(t *testing.T) {
	got := dovi.CRC32MPEG2([]byte("123456789"))
	if got != 0x0376e6e7 {
		t.Fatalf("CRC = %08x", got)
	}
}

func TestParseGeneratedProfiles(t *testing.T) {
	tests := []struct {
		profile int
		want    int
	}{
		{5, 5},
		{7, 7},
		{8, 8},
		{4, 4},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("profile%d", tc.profile), func(t *testing.T) {
			data := testmedia.MakeRPU(testmedia.RPUOptions{Profile: tc.profile})
			rpu, err := dovi.ParseRPU(data)
			if err != nil {
				t.Fatal(err)
			}
			if rpu.Profile != tc.want || !rpu.CRCValid {
				t.Fatalf("RPU = %+v", rpu)
			}
		})
	}
}

func TestRPUReferenceFields(t *testing.T) {
	first := testmedia.MakeRPU(testmedia.RPUOptions{Profile: 8, MappingID: 3})
	next := testmedia.MakeRPU(testmedia.RPUOptions{Profile: 8, UsePrevious: true, PreviousID: 3})

	a, err := dovi.ParseRPU(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := dovi.ParseRPU(next)
	if err != nil {
		t.Fatal(err)
	}
	if a.UsePreviousRPU || a.MappingID != 3 {
		t.Fatalf("first = %+v", a)
	}
	if !b.UsePreviousRPU || b.PreviousRPUID != 3 {
		t.Fatalf("next = %+v", b)
	}
}

func TestRPUCRCFailureIsReported(t *testing.T) {
	data := testmedia.MakeRPU(testmedia.RPUOptions{Profile: 8})
	data[len(data)-5] ^= 0xff
	rpu, err := dovi.ParseRPU(data)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("CRC")) {
		t.Fatalf("err = %v, rpu = %+v", err, rpu)
	}
	if rpu == nil || rpu.CRCValid {
		t.Fatalf("rpu = %+v", rpu)
	}
}
