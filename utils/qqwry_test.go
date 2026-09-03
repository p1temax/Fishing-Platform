package utils

import (
	"encoding/binary"
	"testing"
)

func TestQQWryFind(t *testing.T) {
	data := make([]byte, 64)
	binary.LittleEndian.PutUint32(data[0:4], 8)
	binary.LittleEndian.PutUint32(data[4:8], 8)
	binary.LittleEndian.PutUint32(data[8:12], 0x01010100)
	data[12] = 15
	binary.LittleEndian.PutUint32(data[15:19], 0x010101ff)
	copy(data[19:], []byte("Country\x00Area\x00"))

	db, err := NewQQWry(data)
	if err != nil {
		t.Fatalf("NewQQWry() error = %v", err)
	}

	location, ok := db.Find("1.1.1.1")
	if !ok {
		t.Fatalf("Find() did not match IP range")
	}
	if got, want := location.String(), "Country Area"; got != want {
		t.Fatalf("location.String() = %q, want %q", got, want)
	}

	if _, ok := db.Find("2.2.2.2"); ok {
		t.Fatalf("Find() matched IP outside range")
	}
}

func TestCleanQQWryText(t *testing.T) {
	if got := cleanQQWryText("CZ88.NET"); got != "" {
		t.Fatalf("cleanQQWryText() = %q, want empty", got)
	}
}
